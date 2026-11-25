package topolib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

type fsUpdater struct {
	OfflineProvider

	ctx       context.Context
	ctxCancel context.CancelFunc
	logger    Logger
	fs        fsDir
	stats     *UsageStats
}

type updateProgressLoggerSetter interface {
	SetUpdateProgressLogger(string, UpdateProgressLogger)
}

func (f *fsUpdater) Start() error {
	targetDir, targetTime, err := f.fs.GetTargetDir()
	if err != nil {
		return fmt.Errorf("cannot get target dir: %w", err)
	}

	if err := f.fs.Cleanup(targetDir); err != nil {
		return fmt.Errorf("cannot do startup cleanup: %w", err)
	}

	if targetDir != "" {
		if err := f.Open(targetDir); err == nil {
			f.stats.notifyUpdated(targetTime)

			go f.runBgUpdate()

			return nil
		}
	}

	if err := f.fs.Cleanup(); err != nil {
		return fmt.Errorf("cannot make full cleanup: %w", err)
	}

	go f.runBgUpdate()

	return nil
}

func (f *fsUpdater) Shutdown() {
	f.ctxCancel()
	f.OfflineProvider.Shutdown()
}

func (f *fsUpdater) runBgUpdate() {
	_, modTime, _ := f.fs.GetTargetDir()
	duration := f.UpdateEvery() - time.Since(modTime)

	if duration > 0 {
		timer := time.NewTimer(duration)
		defer func() {
			timer.Stop()

			select {
			case <-timer.C:
			default:
			}
		}()

		select {
		case <-f.ctx.Done():
			return
		case <-timer.C:
		}
	}

	if err := f.doUpdate(); err != nil {
		f.logger.UpdateError(f.Name(), err)
	} else {
		f.logger.UpdateInfo(f.Name())
	}

	ticker := time.NewTicker(f.UpdateEvery())
	defer func() {
		ticker.Stop()

		select {
		case <-ticker.C:
		default:
		}
	}()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			if err := f.doUpdate(); err != nil {
				f.logger.UpdateError(f.Name(), err)
			} else {
				f.logger.UpdateInfo(f.Name())
			}
		}
	}
}

func (f *fsUpdater) doUpdate() (retErr error) {
	tmpDir, err := f.fs.TempDir()
	if err != nil {
		return fmt.Errorf("cannot make a temporary dir: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("cannot remove temporary directory %s: %w", tmpDir, err))
		}
	}()

	f.logProgress("download_started", map[string]string{"target_directory": tmpDir})
	downloadCtx := f.ctx
	if progressLogger, ok := f.logger.(UpdateProgressLogger); ok {
		downloadCtx = WithUpdateProgressLogger(downloadCtx, f.Name(), progressLogger)
	}

	if err := f.Download(downloadCtx, tmpDir); err != nil {
		return fmt.Errorf("cannot download databases: %w", err)
	}
	f.logProgress("download_complete", map[string]string{"target_directory": tmpDir})

	f.logProgress("promotion_started", map[string]string{"source_directory": tmpDir})
	newTargetDir, needToReopen, err := f.fs.Promote(tmpDir)
	if err != nil {
		return fmt.Errorf("cannot promote tmp dir: %w", err)
	}
	f.logProgress("promotion_complete", map[string]string{
		"target_directory": newTargetDir,
		"new_target":       fmt.Sprintf("%t", needToReopen),
	})

	if needToReopen || !f.IsOpened() {
		f.logProgress("database_open_started", map[string]string{"directory": newTargetDir})
		if err := f.Open(newTargetDir); err != nil {
			if cleanupErr := os.RemoveAll(newTargetDir); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("cannot remove new target directory %s: %w", newTargetDir, cleanupErr))
			}

			return fmt.Errorf("cannot open a new target dir: %w", err)
		}
		f.logProgress("database_open_complete", map[string]string{"directory": newTargetDir})
		f.logProgress("database_switched", map[string]string{"directory": newTargetDir})
	} else {
		f.logProgress("database_unchanged", map[string]string{"directory": newTargetDir})
	}

	f.logProgress("cleanup_started", map[string]string{"active_directory": newTargetDir})
	if err := f.fs.Cleanup(newTargetDir); err != nil {
		return fmt.Errorf("cannot clean old target directories: %w", err)
	}
	f.logProgress("cleanup_complete", map[string]string{"active_directory": newTargetDir})

	f.stats.notifyUpdated(time.Now())

	return nil
}

func (f *fsUpdater) logProgress(step string, details map[string]string) {
	if logger, ok := f.logger.(UpdateProgressLogger); ok {
		logger.UpdateProgress(f.Name(), step, details)
	}
}

func newFsUpdater(provider OfflineProvider, logger Logger, stats *UsageStats) (OfflineProvider, error) {
	ctx, cancel := context.WithCancel(context.Background())

	updater := &fsUpdater{
		OfflineProvider: provider,
		ctx:             ctx,
		ctxCancel:       cancel,
		logger:          logger,
		fs:              fsDir{Dir: provider.BaseDirectory()},
		stats:           stats,
	}

	if progressLogger, ok := logger.(UpdateProgressLogger); ok {
		if setter, ok := provider.(updateProgressLoggerSetter); ok {
			setter.SetUpdateProgressLogger(provider.Name(), progressLogger)
		}
	}

	if err := updater.Start(); err != nil {
		return nil, fmt.Errorf("cannot start fs updater for provider %s: %w",
			provider.Name(),
			err)
	}

	return updater, nil
}
