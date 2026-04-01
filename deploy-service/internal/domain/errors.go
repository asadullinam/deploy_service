package domain

import "errors"

var (
	ErrProjectNotFound               = errors.New("project not found")
	ErrProjectEnvironmentUnavailable = errors.New("project environment is unavailable")
	ErrLogsUnavailable               = errors.New("project logs are unavailable")
)
