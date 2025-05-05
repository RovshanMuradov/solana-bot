// internal/bot/tasks.go
package bot

//
//func (r *Runner) handleSnipeTask(ctx context.Context, t *task.Task, dexAdapter dex.DEX, dexTask *dex.Task, logger *zap.Logger) {
//	logger.Info("Starting snipe operation",
//		zap.String("task", t.TaskName),
//		zap.String("token", t.TokenMint),
//		zap.Float64("amount_sol", dexTask.AmountSol),
//		zap.String("dex", dexAdapter.GetName()))
//
//	opCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
//	defer cancel()
//
//	if err := dexAdapter.Execute(opCtx, dexTask); err != nil {
//		logger.Error("Snipe operation failed", zap.String("task", t.TaskName), zap.Error(err))
//		return
//	}
//	logger.Info("Snipe operation completed successfully", zap.String("task", t.TaskName))
//
//	time.Sleep(5 * time.Second)
//
//	monitorConfig := &monitor.SessionConfig{
//		TokenMint:       t.TokenMint,
//		InitialAmount:   dexTask.AmountSol,
//		MonitorInterval: dexTask.MonitorInterval,
//		DEX:             dexAdapter,
//		Logger:          logger.Named("monitor"),
//		SlippagePercent: dexTask.SlippagePercent,
//		PriorityFee:     dexTask.PriorityFee,
//		ComputeUnits:    dexTask.ComputeUnits,
//	}
//
//	session := monitor.NewMonitoringSession(monitorConfig)
//	if err := session.Start(); err != nil {
//		logger.Error("Failed to start monitoring session", zap.String("task", t.TaskName), zap.Error(err))
//		return
//	}
//
//	logger.Info("Monitoring session started - press Enter to sell tokens or 'q' to exit", zap.String("task", t.TaskName))
//
//	if err := session.Wait(); err != nil {
//		logger.Error("Monitoring session error", zap.String("task", t.TaskName), zap.Error(err))
//	}
//}
