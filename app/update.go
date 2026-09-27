package app

import (
	"context"
	"errors"
	"time"

	"github.com/BennyThink/NFCX/internal/update"
)

func (s *Service) UpdateStatus() update.Snapshot {
	if s.updater == nil {
		return update.Snapshot{State: update.StateUnavailable, Detail: "暂时无法检查更新"}
	}
	return s.updateSnapshot(s.updater.Snapshot())
}

func (s *Service) CheckForUpdates() (update.Snapshot, error) {
	if s.updater == nil {
		return s.UpdateStatus(), update.ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snapshot, err := s.updater.Check(ctx)
	return s.updateSnapshot(snapshot), err
}

// CheckUpdatesInBackground is intentionally quiet: UI callers receive a
// state event only after a due check has completed, never a modal prompt.
func (s *Service) CheckUpdatesInBackground() {
	if s.updater == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		snapshot, checked, _ := s.updater.CheckIfDue(ctx)
		if checked {
			s.emitter.Emit(UpdateEventName, s.updateSnapshot(snapshot))
		}
	}()
}

func (s *Service) DownloadUpdate() (update.Snapshot, error) {
	if s.updater == nil {
		return s.UpdateStatus(), update.ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	snapshot, err := s.updater.Download(ctx)
	return s.updateSnapshot(snapshot), err
}

func (s *Service) SetAutomaticUpdateDownload(enabled bool) (update.Snapshot, error) {
	if s.updater == nil {
		return s.UpdateStatus(), update.ErrNotConfigured
	}
	if err := s.updater.SetAutoDownload(enabled); err != nil {
		return s.UpdateStatus(), err
	}
	return s.updateSnapshot(s.updater.Snapshot()), nil
}

// RestartAndApply intentionally refuses unsafe states. The platform helper is
// introduced alongside release packaging; until it is bundled this keeps the
// already verified artifact and reports a precise limitation.
func (s *Service) RestartAndApplyUpdate() error {
	s.mu.Lock()
	busy := len(s.operations) != 0
	s.mu.Unlock()
	if busy {
		return errors.New("wait for the active NFCX task to finish before installing the update")
	}
	if s.bench != nil && s.bench.Snapshot().Dirty {
		return errors.New("save or discard the working copy before installing the update")
	}
	snapshot := s.UpdateStatus()
	if snapshot.State != update.StateReady {
		return errors.New("no verified update is ready")
	}
	if !snapshot.CanApply {
		return errors.New("this installation format requires manual installation")
	}
	return s.updater.StartApply()
}

func (s *Service) updateSnapshot(snapshot update.Snapshot) update.Snapshot {
	s.mu.Lock()
	busy := len(s.operations) != 0
	s.mu.Unlock()
	if busy || (s.bench != nil && s.bench.Snapshot().Dirty) {
		snapshot.CanApply = false
	}
	return snapshot
}
