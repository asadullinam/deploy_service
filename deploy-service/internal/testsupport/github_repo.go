package testsupport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func NewTempGitHubRepo(ctx context.Context, dir, owner, name, defaultBranch string, initialFiles map[string]string) (*TempGitHubRepo, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create repo dir: %w", err)
	}

	repo := &TempGitHubRepo{
		Dir:           dir,
		Owner:         owner,
		Name:          name,
		DefaultBranch: defaultBranch,
	}

	if err := repo.runGit(ctx, "init", "-b", defaultBranch); err != nil {
		return nil, err
	}
	if err := repo.runGit(ctx, "config", "user.email", "test@example.com"); err != nil {
		return nil, err
	}
	if err := repo.runGit(ctx, "config", "user.name", "Deploy Service Test"); err != nil {
		return nil, err
	}

	for path, content := range initialFiles {
		if err := repo.writeWorkingTreeFile(path, content); err != nil {
			return nil, err
		}
	}
	if err := repo.runGit(ctx, "add", "."); err != nil {
		return nil, err
	}
	if err := repo.runGit(ctx, "commit", "-m", "Initial commit"); err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *TempGitHubRepo) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, "/repos/") {
		http.NotFound(w, req)
		return
	}

	path := strings.TrimPrefix(req.URL.Path, "/repos/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, req)
		return
	}
	owner, repoName := parts[0], parts[1]
	if owner != r.Owner || repoName != r.Name {
		http.NotFound(w, req)
		return
	}

	rest := parts[2:]
	switch {
	case len(rest) == 0 && req.Method == http.MethodGet:
		r.writeJSON(w, http.StatusOK, map[string]any{
			"default_branch": r.DefaultBranch,
		})
	case len(rest) >= 4 && rest[0] == "git" && rest[1] == "ref" && rest[2] == "heads" && req.Method == http.MethodGet:
		branch := strings.Join(rest[3:], "/")
		sha, err := r.branchSHA(req.Context(), branch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]any{
			"object": map[string]string{"sha": sha},
		})
	case len(rest) == 2 && rest[0] == "git" && rest[1] == "refs" && req.Method == http.MethodPost:
		var payload struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
		if err := r.createBranch(req.Context(), branch, payload.SHA); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.writeJSON(w, http.StatusCreated, map[string]string{"ref": payload.Ref})
	case len(rest) >= 2 && rest[0] == "contents" && req.Method == http.MethodGet:
		filePath := strings.Join(rest[1:], "/")
		branch := req.URL.Query().Get("ref")
		if branch == "" {
			branch = r.DefaultBranch
		}
		content, sha, err := r.readFile(req.Context(), branch, filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		r.writeJSON(w, http.StatusOK, map[string]string{
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte(content)),
			"sha":      sha,
		})
	case len(rest) >= 2 && rest[0] == "contents" && req.Method == http.MethodPut:
		filePath := strings.Join(rest[1:], "/")
		var payload struct {
			Message string `json:"message"`
			Content string `json:"content"`
			Branch  string `json:"branch"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(payload.Content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := r.upsertFile(req.Context(), payload.Branch, filePath, string(decoded), payload.Message); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.writeJSON(w, http.StatusCreated, map[string]string{"content": filePath})
	case len(rest) == 1 && rest[0] == "pulls" && req.Method == http.MethodPost:
		r.mu.Lock()
		r.pullRequests++
		number := r.pullRequests
		r.mu.Unlock()
		r.writeJSON(w, http.StatusCreated, map[string]string{
			"html_url": fmt.Sprintf("https://fake.github.local/%s/%s/pull/%d", r.Owner, r.Name, number),
		})
	case len(rest) == 1 && rest[0] == "hooks" && req.Method == http.MethodPost:
		r.writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	default:
		http.NotFound(w, req)
	}
}

func (r *TempGitHubRepo) BranchExists(ctx context.Context, branch string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runGit(ctx, "rev-parse", "--verify", branch) == nil
}

func (r *TempGitHubRepo) ReadFile(ctx context.Context, branch, path string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	content, _, err := r.readFileLocked(ctx, branch, path)
	return content, err
}

func (r *TempGitHubRepo) MergeBranchIntoDefault(ctx context.Context, branch string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.runGit(ctx, "checkout", r.DefaultBranch); err != nil {
		return err
	}
	if err := r.runGit(ctx, "merge", "--no-ff", branch, "-m", "Merge test branch"); err != nil {
		return err
	}
	return nil
}

func (r *TempGitHubRepo) ValidateGeneratedArtifacts(ctx context.Context, branch, projectID, serviceName string) error {
	workflow, err := r.ReadFile(ctx, branch, ".github/workflows/deploy-service.yml")
	if err != nil {
		return err
	}
	if !strings.Contains(workflow, "STAGE_SLUG: ${{ vars.STAGE_SLUG || 'production' }}") {
		return fmt.Errorf("workflow does not define stage namespace configuration for project %s", projectID)
	}
	if !strings.Contains(workflow, `kubectl -n "$STAGE_SLUG"`) {
		return fmt.Errorf("workflow does not apply manifests into STAGE_SLUG for project %s", projectID)
	}

	deploymentPath := filepath.ToSlash(filepath.Join("k8s", serviceName, "deployment.yaml"))
	deployment, err := r.ReadFile(ctx, branch, deploymentPath)
	if err != nil {
		return err
	}
	if !strings.Contains(deployment, "kind: Deployment") || !strings.Contains(deployment, "IMAGE_PLACEHOLDER") {
		return fmt.Errorf("deployment manifest is incomplete")
	}

	servicePath := filepath.ToSlash(filepath.Join("k8s", serviceName, "service.yaml"))
	serviceYAML, err := r.ReadFile(ctx, branch, servicePath)
	if err != nil {
		return err
	}
	if !strings.Contains(serviceYAML, "kind: Service") {
		return fmt.Errorf("service manifest is incomplete")
	}

	return nil
}

func (r *TempGitHubRepo) branchSHA(ctx context.Context, branch string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.branchSHALocked(ctx, branch)
}

func (r *TempGitHubRepo) branchSHALocked(ctx context.Context, branch string) (string, error) {
	out, err := r.outputGit(ctx, "rev-parse", branch)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *TempGitHubRepo) createBranch(ctx context.Context, branch string, sha string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runGit(ctx, "branch", branch, sha)
}

func (r *TempGitHubRepo) readFile(ctx context.Context, branch, path string) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readFileLocked(ctx, branch, path)
}

func (r *TempGitHubRepo) readFileLocked(ctx context.Context, branch, path string) (string, string, error) {
	content, err := r.outputGit(ctx, "show", fmt.Sprintf("%s:%s", branch, filepath.ToSlash(path)))
	if err != nil {
		return "", "", err
	}
	sha, err := r.outputGit(ctx, "rev-parse", fmt.Sprintf("%s:%s", branch, filepath.ToSlash(path)))
	if err != nil {
		return "", "", err
	}
	return content, strings.TrimSpace(sha), nil
}

func (r *TempGitHubRepo) upsertFile(ctx context.Context, branch, path, content, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if branch == "" {
		branch = r.DefaultBranch
	}
	if err := r.runGit(ctx, "checkout", branch); err != nil {
		return err
	}
	if err := r.writeWorkingTreeFile(path, content); err != nil {
		return err
	}
	if err := r.runGit(ctx, "add", filepath.ToSlash(path)); err != nil {
		return err
	}
	status, err := r.outputGit(ctx, "status", "--porcelain", "--", filepath.ToSlash(path))
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if message == "" {
		message = "Update file"
	}
	return r.runGit(ctx, "commit", "-m", message)
}

func (r *TempGitHubRepo) writeWorkingTreeFile(path, content string) error {
	absolutePath := filepath.Join(r.Dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	return nil
}

func (r *TempGitHubRepo) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (r *TempGitHubRepo) outputGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (r *TempGitHubRepo) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
