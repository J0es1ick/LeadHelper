package integrations

import (
	"context"
	"leadhelper/internal/domain"
)

func (p LiveJira) CreationProjects(ctx context.Context) ([]domain.JiraOption, error) {
	return p.Hub.jira(p.Hub.config("jira")).CreationProjects(ctx)
}
func (p LiveJira) CreationTypes(ctx context.Context, project string) ([]domain.JiraOption, error) {
	return p.Hub.jira(p.Hub.config("jira")).CreationTypes(ctx, project)
}
func (p LiveJira) CreationFields(ctx context.Context, project, typ string) (domain.JiraCreateMeta, error) {
	return p.Hub.jira(p.Hub.config("jira")).CreationFields(ctx, project, typ)
}
func (p LiveJira) AssignableUsers(ctx context.Context, project, search string) ([]domain.JiraUser, error) {
	return p.Hub.jira(p.Hub.config("jira")).AssignableUsers(ctx, project, search)
}
func (p LiveJira) CreateIssue(ctx context.Context, in domain.JiraCreateInput) (domain.JiraCreateResult, error) {
	return p.Hub.jira(p.Hub.config("jira")).CreateIssue(ctx, in)
}
func (p LiveJira) UploadIssueFiles(ctx context.Context, key string, files []domain.JiraUpload) []string {
	return p.Hub.jira(p.Hub.config("jira")).UploadIssueFiles(ctx, key, files)
}
func (p LiveJira) FetchWorklogs(ctx context.Context, members []domain.Member, from, to string) ([]domain.Worklog, error) {
	return p.Hub.jira(p.Hub.config("jira")).FetchWorklogs(ctx, members, from, to)
}
