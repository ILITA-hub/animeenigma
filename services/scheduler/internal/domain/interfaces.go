package domain

import "context"

// TaskProcessor defines the interface for processing anime load tasks
type TaskProcessor interface {
	ProcessTask(ctx context.Context, task *AnimeLoadTask) error
	CheckExportJobCompletion(ctx context.Context, exportJobID string) error
}
