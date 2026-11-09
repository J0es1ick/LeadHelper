package main

import (
	"context"
	"leadhelper/internal/adapters/httpapi"
	"leadhelper/internal/adapters/integrations"
	"leadhelper/internal/adapters/mailformat"
	"leadhelper/internal/adapters/storage"
	"leadhelper/internal/application"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	dataFile := env("DATA_FILE", "data/leadhelper.json")
	repo, e := storage.OpenSQLite(dataFile+".sqlite", dataFile, application.Seed(time.Now()))
	if e != nil {
		slog.Error("database open failed", "error", e)
		os.Exit(1)
	}
	defer repo.Close()
	vault := storage.Secrets{Path: filepath.Join(filepath.Dir(dataFile), "integration-secrets.bin")}
	secrets, e := vault.Load()
	if e != nil {
		slog.Error("integration secrets could not be loaded", "error", e)
		os.Exit(1)
	}
	hub, e := integrations.NewHub(map[string]domain.IntegrationConfig{
		"jira":       {URL: os.Getenv("JIRA_URL"), Email: os.Getenv("JIRA_EMAIL"), Token: os.Getenv("JIRA_API_TOKEN"), JQL: os.Getenv("JIRA_JQL"), Mode: env("JIRA_MODE", "server")},
		"bitrix":     {URL: os.Getenv("BITRIX_WEBHOOK_URL")},
		"confluence": {URL: os.Getenv("CONFLUENCE_URL"), Token: os.Getenv("CONFLUENCE_TOKEN")},
	}, secrets, vault.Save)
	if e != nil {
		slog.Error("integration configuration failed", "error", e)
		os.Exit(1)
	}
	sources := map[string]ports.TaskSource{"jira": integrations.LiveJira{Hub: hub}}
	var mail ports.MailProvider
	if os.Getenv("MAIL_PROVIDER") == "graph" {
		mail = &integrations.Graph{Token: os.Getenv("MS_GRAPH_ACCESS_TOKEN"), AllowSend: os.Getenv("ALLOW_LIVE_MAIL_SEND") == "true"}
	} else {
		password := os.Getenv("IMAP_PASSWORD")
		if path := os.Getenv("IMAP_PASSWORD_FILE"); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				slog.Error("IMAP_PASSWORD_FILE could not be read")
				os.Exit(1)
			}
			password = strings.TrimRight(string(raw), "\r\n")
		}
		port, err := strconv.Atoi(env("IMAP_PORT", "993"))
		if err != nil || port < 1 || port > 65535 {
			slog.Error("invalid IMAP_PORT")
			os.Exit(1)
		}
		var folders []string
		if value := os.Getenv("MAIL_FOLDERS"); value != "" {
			for _, f := range strings.Split(value, "|") {
				if f = strings.TrimSpace(f); f != "" {
					folders = append(folders, f)
				}
			}
		}
		smtpHost := os.Getenv("SMTP_HOST")
		if smtpHost == "" && strings.EqualFold(os.Getenv("IMAP_HOST"), "imap.timeweb.ru") {
			smtpHost = "smtp.timeweb.ru"
		}
		smtpPort, err := strconv.Atoi(env("SMTP_PORT", "465"))
		if err != nil || smtpPort < 1 || smtpPort > 65535 {
			slog.Error("invalid SMTP_PORT")
			os.Exit(1)
		}
		smtpPassword := env("SMTP_PASSWORD", password)
		if path := os.Getenv("SMTP_PASSWORD_FILE"); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				slog.Error("SMTP_PASSWORD_FILE could not be read")
				os.Exit(1)
			}
			smtpPassword = strings.TrimRight(string(raw), "\r\n")
		}
		smtp := &integrations.SMTP{Host: smtpHost, Port: smtpPort, User: env("SMTP_USER", os.Getenv("IMAP_USER")), Password: smtpPassword, Security: env("SMTP_SECURITY", "tls")}
		if os.Getenv("ALLOW_LIVE_MAIL_SEND") == "false" {
			smtp = nil
		}
		mail = &integrations.IMAP{Host: strings.TrimSpace(os.Getenv("IMAP_HOST")), Port: port, User: os.Getenv("IMAP_USER"), Password: password, Folders: folders, SMTP: smtp}
	}
	service := application.New(repo, sources, mail).WithBlobs(&storage.Blobs{Directory: filepath.Join(filepath.Dir(dataFile), "mail-blobs")}).WithWorkspace(integrations.LiveBitrix{Hub: hub}, integrations.LiveConfluence{Hub: hub}, hub)
	service.WithProjectSource(integrations.LiveProjectSheets{Hub: hub})
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(dataFile), "projects-sheet-bootstrap.json")); err == nil {
		if err = service.BootstrapProjects(context.Background(), raw); err != nil {
			slog.Error("project import failed", "error", err)
		}
	}
	if err := service.UpgradeTeamPlans(context.Background()); err != nil {
		slog.Error("team plan upgrade failed", "error", err)
		os.Exit(1)
	}
	if err := service.UpgradeIntegrations(context.Background()); err != nil {
		slog.Error("integration upgrade failed", "error", err)
		os.Exit(1)
	}
	if err := service.UpgradeMail(context.Background(), mailformat.Decode); err != nil {
		slog.Error("mail format upgrade failed", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: env("HTTP_ADDR", "127.0.0.1:8090"), Handler: httpapi.New(service, env("STATIC_DIR", "../frontend/dist")), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	interval, err := time.ParseDuration(env("MAIL_SYNC_INTERVAL", "20s"))
	if err != nil || interval < 10*time.Second {
		slog.Error("MAIL_SYNC_INTERVAL must be at least 10s")
		os.Exit(1)
	}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); service.RunMailWorker(ctx, interval) }()
	workspaceDone := make(chan struct{})
	go func() { defer close(workspaceDone); service.RunWorkspaceWorker(ctx) }()
	projectsDone := make(chan struct{})
	go func() { defer close(projectsDone); service.RunProjectWorker(ctx) }()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("LeadHelper started", "address", server.Addr, "demo", "switchable in Settings")
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		slog.Error("server failed", "error", e)
		os.Exit(1)
	}
	stop()
	service.StopExports()
	<-workerDone
	<-workspaceDone
	<-projectsDone
}
