// Package casetasks lists every task of a case, whatever its read roles.
//
// core's TaskStore lists tasks only through GetTasks, which returns a task only to a
// caller holding one of its read.roles. The listings here are not a caller's: the
// consignment detail and the agency case view authorize the caller on the case
// first, and plugins and admin views read the case as the system. So they read the
// task table by case directly, as they did through core's former GetAllTasks.
package casetasks

import (
	"context"
	"fmt"

	"github.com/OpenNSW/core/taskflow/store"
	gormstore "github.com/OpenNSW/core/taskflow/store/gorm"
	"gorm.io/gorm"
)

// Store is core's gorm task store with a listing of a case's tasks.
type Store struct {
	*gormstore.TaskStore
	db *gorm.DB
}

// New returns a Store backed by db.
func New(db *gorm.DB) *Store {
	return &Store{TaskStore: gormstore.New(db), db: db}
}

// TasksOfCase returns every task of caseID (store.TaskRecord.CaseID), across every
// workflow of the case, or the error that kept them from being read. ReadRoles is
// left empty, as core's read methods leave it.
func (s *Store) TasksOfCase(ctx context.Context, caseID string) ([]store.TaskRecord, error) {
	var models []gormstore.TaskRecordModel
	if err := s.db.WithContext(ctx).Where("case_id = ?", caseID).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list the tasks of case %s: %w", caseID, err)
	}
	records := make([]store.TaskRecord, len(models))
	for i, m := range models {
		records[i] = m.ToDomain()
	}
	return records, nil
}
