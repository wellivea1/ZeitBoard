package main

import (
	"context"
	storage "non24.app/core/storage/sqlite"
)

type TaskConflictVersionDTO struct {
	ChoiceID string  `json:"choiceId"`
	Task     TaskDTO `json:"task"`
}
type TaskConflictDTO struct {
	TaskID      string                   `json:"taskId"`
	ReviewToken string                   `json:"reviewToken"`
	Local       TaskDTO                  `json:"local"`
	Downloaded  []TaskConflictVersionDTO `json:"downloaded"`
}
type TaskConflictHistoryDTO struct {
	ReviewToken  string          `json:"reviewToken"`
	TaskID       string          `json:"taskId"`
	Choice       string          `json:"choice"`
	DecidedLabel string          `json:"decidedLabel"`
	Result       TaskDTO         `json:"result"`
	Before       TaskConflictDTO `json:"before"`
}
type ResolveTaskConflictInput struct {
	TaskID      string `json:"taskId"`
	ReviewToken string `json:"reviewToken"`
	ChoiceID    string `json:"choiceId"`
}

func taskConflictDTO(record storage.TaskSyncConflict) TaskConflictDTO {
	result := TaskConflictDTO{TaskID: record.TaskID, ReviewToken: record.ReviewToken, Local: taskDTO(record.Local), Downloaded: []TaskConflictVersionDTO{}}
	for _, version := range record.Downloaded {
		result.Downloaded = append(result.Downloaded, TaskConflictVersionDTO{ChoiceID: version.ChoiceID, Task: taskDTO(version.Task)})
	}
	return result
}
func (a *App) attachTaskConflictReview(dto *ProposalsDTO) error {
	dto.TaskConflicts = []TaskConflictDTO{}
	dto.TaskConflictHistory = []TaskConflictHistoryDTO{}
	store, err := a.requireStore()
	if err != nil {
		return err
	}
	conflicts, err := store.ListTaskSyncConflicts(context.Background())
	if err != nil {
		return err
	}
	for _, conflict := range conflicts {
		dto.TaskConflicts = append(dto.TaskConflicts, taskConflictDTO(conflict))
	}
	history, err := store.ListTaskSyncResolutions(context.Background())
	if err != nil {
		return err
	}
	for _, record := range history {
		choice := "Downloaded version"
		if record.ChosenID == "local" {
			choice = "Local version"
		}
		dto.TaskConflictHistory = append(dto.TaskConflictHistory, TaskConflictHistoryDTO{ReviewToken: record.ReviewToken, TaskID: record.TaskID, Choice: choice, DecidedLabel: record.DecidedAt.Local().Format("Jan 2, 2006, 3:04 PM"), Result: taskDTO(record.Result), Before: taskConflictDTO(record.Before)})
	}
	return nil
}

// ResolveTaskConflict is an owner-only review action. It does not expose an agent
// approval endpoint and never changes a calendar placement.
func (a *App) ResolveTaskConflict(input ResolveTaskConflictInput) (TaskConflictHistoryDTO, error) {
	store, err := a.requireStore()
	if err != nil {
		return TaskConflictHistoryDTO{}, err
	}
	result, err := store.ResolveTaskSyncConflict(a.applicationContext(), input.TaskID, input.ReviewToken, input.ChoiceID, a.currentTime().UTC())
	if err != nil {
		return TaskConflictHistoryDTO{}, err
	}
	choice := "Downloaded version"
	if result.ChosenID == "local" {
		choice = "Local version"
	}
	return TaskConflictHistoryDTO{ReviewToken: result.ReviewToken, TaskID: result.TaskID, Choice: choice, DecidedLabel: result.DecidedAt.Local().Format("Jan 2, 2006, 3:04 PM"), Result: taskDTO(result.Result), Before: taskConflictDTO(result.Before)}, nil
}
