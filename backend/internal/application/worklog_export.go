package application

import (
	"context"
	"fmt"
	"leadhelper/internal/domain"
	"leadhelper/internal/ports"
	"math"
	"sort"
	"strings"
	"time"
)

type worklogExportJob struct {
	active  bool
	status  domain.WorklogExportStatus
	report  domain.WorklogExportReport
	created time.Time
	cancel  context.CancelFunc
}

func (s *Service) exportRoster(ctx context.Context, ids []string) (ports.JiraWorklogSource, []domain.Member, []domain.Member, string, error) {
	if len(ids) == 0 || len(ids) > 500 {
		return nil, nil, nil, "", invalid("Выберите от 1 до 500 сотрудников")
	}
	if e := s.workspaceEnabled(ctx, "team"); e != nil {
		return nil, nil, nil, "", e
	}
	p, _, mapped, _, _, e := s.worklogContext(ctx)
	if e != nil {
		return nil, nil, nil, "", e
	}
	st, e := s.taskShell(ctx)
	if e != nil {
		return nil, nil, nil, "", e
	}
	all := map[string]domain.Member{}
	for _, m := range st.Members {
		if !m.Demo && !m.Inactive {
			all[m.ID] = m
		}
	}
	linked := map[string]domain.Member{}
	for _, m := range mapped {
		linked[m.ID] = m
	}
	selected, available := []domain.Member{}, []domain.Member{}
	seen := map[string]bool{}
	for _, id := range ids {
		m, ok := all[id]
		if !ok || seen[id] {
			return nil, nil, nil, "", invalid("Состав команды изменился или сотрудник выбран повторно. Проверьте выбор")
		}
		seen[id] = true
		selected = append(selected, m)
		if value, ok := linked[id]; ok {
			available = append(available, value)
		}
	}
	if len(available) == 0 {
		return nil, nil, nil, "", invalid("У выбранных сотрудников нет сопоставления с Jira. Настройте его в интеграциях")
	}
	sort.Slice(selected, func(i, j int) bool { return strings.ToLower(selected[i].Name) < strings.ToLower(selected[j].Name) })
	return p, selected, available, p.SiteID() + ":" + jiraMembersFingerprint(selected), nil
}

func (s *Service) StartWorklogExport(ctx context.Context, in domain.WorklogExportRequest) (domain.WorklogExportStatus, error) {
	periods, e := domain.WorklogExportPeriods(in)
	if e != nil {
		return domain.WorklogExportStatus{}, invalid(e.Error())
	}
	p, members, mapped, fingerprint, e := s.exportRoster(ctx, in.MemberIDs)
	if e != nil {
		return domain.WorklogExportStatus{}, e
	}
	from, _ := time.Parse("2006-01-02", in.From)
	to, _ := time.Parse("2006-01-02", in.To)
	days := int(to.Sub(from)/(24*time.Hour)) + 1
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	if s.exportClosed {
		return domain.WorklogExportStatus{}, invalid("Сервис перезапускается. Повторите выгрузку после запуска")
	}
	if s.exportJobs == nil {
		s.exportJobs = map[string]*worklogExportJob{}
	}
	for id, job := range s.exportJobs {
		if job.active {
			return domain.WorklogExportStatus{}, invalid("Выгрузка уже выполняется. Дождитесь её завершения или отмените")
		}
		if time.Since(job.created) > 30*time.Minute {
			delete(s.exportJobs, id)
		}
	}
	for len(s.exportJobs) >= 5 {
		oldest := ""
		for id, j := range s.exportJobs {
			if oldest == "" || j.created.Before(s.exportJobs[oldest].created) {
				oldest = id
			}
		}
		delete(s.exportJobs, oldest)
	}
	status := domain.WorklogExportStatus{ID: ID("hours-"), State: "running", Chunks: (days + 30) / 31, Employees: len(members), Missing: len(members) - len(mapped)}
	run, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	job := &worklogExportJob{status: status, created: time.Now(), cancel: cancel, active: true}
	s.exportJobs[status.ID] = job
	s.exportWG.Add(1)
	go func() {
		defer s.exportWG.Done()
		defer cancel()
		defer func() {
			if recover() != nil {
				s.finishWorklogExport(job, domain.WorklogExportReport{}, fmt.Errorf("Не удалось сформировать отчёт"))
			}
		}()
		report, e := s.collectWorklogExport(run, in, periods, p, members, mapped, fingerprint, func(done int) { s.exportMu.Lock(); job.status.Done = done; s.exportMu.Unlock() })
		s.finishWorklogExport(job, report, e)
	}()
	return status, nil
}
func (s *Service) finishWorklogExport(job *worklogExportJob, report domain.WorklogExportReport, e error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	job.active = false
	if job.status.State == "cancelled" {
		return
	}
	if e != nil {
		job.status.State = "error"
		job.status.Error = e.Error()
		return
	}
	job.report = report
	job.status.State = "ready"
}
func (s *Service) collectWorklogExport(ctx context.Context, in domain.WorklogExportRequest, periods []domain.WorklogPeriod, p ports.JiraWorklogSource, members, mapped []domain.Member, fingerprint string, progress func(int)) (domain.WorklogExportReport, error) {
	out := domain.WorklogExportReport{From: in.From, To: in.To, Periods: periods, Rows: []domain.WorklogExportRow{}, Missing: len(members) - len(mapped)}
	// Keep only the fields needed for totals. A worklog moved during a long export
	// replaces its earlier occurrence instead of being counted twice.
	type entry struct {
		member, date string
		seconds      int64
	}
	records := map[string]entry{}
	allowed := map[string]bool{}
	for _, m := range mapped {
		allowed[m.ID] = true
	}
	from, _ := time.Parse("2006-01-02", in.From)
	to, _ := time.Parse("2006-01-02", in.To)
	chunk := 0
	for start := from; !start.After(to); start = start.AddDate(0, 0, 31) {
		if ctx.Err() != nil {
			return out, invalid("Выгрузка прервана или превысила 10 минут. Выберите меньший период")
		}
		end := start.AddDate(0, 0, 30)
		if end.After(to) {
			end = to
		}
		_, _, _, current, e := s.exportRoster(ctx, in.MemberIDs)
		if e != nil || current != fingerprint {
			return out, invalid("Команда или подключение Jira изменились. Запустите выгрузку заново")
		}
		request, cancel := context.WithTimeout(ctx, 90*time.Second)
		rows, e := p.FetchWorklogs(request, mapped, start.Format("2006-01-02"), end.Format("2006-01-02"))
		cancel()
		if e != nil {
			return out, invalid(fmt.Sprintf("Не удалось загрузить %s — %s: %s. Неполный отчёт не сформирован", start.Format("02.01.2006"), end.Format("02.01.2006"), e.Error()))
		}
		for _, r := range rows {
			if !allowed[r.MemberID] {
				continue
			}
			if r.Date < start.Format("2006-01-02") || r.Date > end.Format("2006-01-02") {
				continue
			}
			if _, e := time.Parse("2006-01-02", r.Date); e != nil || r.ID == "" || r.Seconds < 0 {
				return out, invalid("Jira вернула некорректное списание")
			}
			records[r.ID] = entry{r.MemberID, r.Date, r.Seconds}
			if len(records) > 200000 {
				return out, invalid("Более 200 000 списаний. Уменьшите период или число сотрудников")
			}
		}
		chunk++
		progress(chunk)
	}
	_, _, _, current, e := s.exportRoster(ctx, in.MemberIDs)
	if e != nil || current != fingerprint {
		return out, invalid("Команда или подключение Jira изменились. Запустите выгрузку заново")
	}
	byMember := map[string]int{}
	for _, m := range members {
		byMember[m.ID] = len(out.Rows)
		out.Rows = append(out.Rows, domain.WorklogExportRow{MemberID: m.ID, Name: m.Name, Available: allowed[m.ID], Seconds: make([]int64, len(periods))})
	}
	for _, r := range records {
		index := sort.Search(len(periods), func(i int) bool { return periods[i].To >= r.date })
		if index == len(periods) || r.date < periods[index].From {
			continue
		}
		row := &out.Rows[byMember[r.member]]
		if r.seconds > math.MaxInt64-out.Total {
			return out, invalid("Слишком большая сумма списаний")
		}
		row.Seconds[index] += r.seconds
		row.Total += r.seconds
		out.Total += r.seconds
	}
	out.Records = len(records)
	out.GeneratedAt = time.Now().UTC()
	return out, nil
}
func (s *Service) WorklogExport(id string) (domain.WorklogExportStatus, domain.WorklogExportReport, error) {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	job := s.exportJobs[id]
	if job == nil || time.Since(job.created) > 30*time.Minute {
		return domain.WorklogExportStatus{}, domain.WorklogExportReport{}, ErrNotFound
	}
	return job.status, job.report, nil // Completed reports are immutable.
}
func (s *Service) CancelWorklogExport(id string) error {
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	job := s.exportJobs[id]
	if job == nil {
		return ErrNotFound
	}
	if job.status.State == "running" {
		job.status.State = "cancelled"
		job.cancel()
	}
	return nil
}
func (s *Service) StopExports() {
	s.exportMu.Lock()
	s.exportClosed = true
	for _, job := range s.exportJobs {
		job.cancel()
	}
	s.exportMu.Unlock()
	s.exportWG.Wait()
}
