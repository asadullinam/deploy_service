package testsupport

import "sync"

type TempGitHubRepo struct {
	mu            sync.Mutex
	Dir           string
	Owner         string
	Name          string
	DefaultBranch string
	pullRequests  int
}
