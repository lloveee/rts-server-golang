package sim

// applyCmdTrain processes a CmdTrain command.
// Validation: building owned by player, building type can train requested unit type,
// queue length < MaxQueueLength, player has sufficient crystal.
// Silently drops invalid commands.
func applyCmdTrain(w *World, cmd Cmd) {
	b := w.FindBuilding(cmd.UnitID)
	if b == nil {
		return
	}
	if b.Owner != cmd.Player {
		return
	}
	if b.State != BldReady {
		return
	}
	if len(b.ProductionQueue) >= MaxQueueLength {
		return
	}

	// TargetID encodes the unit type to train.
	unitType := UnitType(cmd.TargetID)
	if unitType < UnitWorker || unitType > UnitCavalry {
		return
	}

	// Validate building can train this unit type.
	stats := BuildingStatTable[b.Type]
	canTrain := false
	for _, ut := range stats.Trains {
		if ut == unitType {
			canTrain = true
			break
		}
	}
	if !canTrain {
		return
	}

	// Validate resource.
	unitStats := UnitStatTable[unitType]
	if int(cmd.Player) >= len(w.Players) {
		return
	}
	if w.Players[cmd.Player].Crystal < unitStats.Cost {
		return
	}

	// Deduct resource and enqueue.
	w.Players[cmd.Player].Crystal = w.Players[cmd.Player].Crystal.Sub(unitStats.Cost)

	b.ProductionQueue = append(b.ProductionQueue, QueueItem{
		UnitType:  unitType,
		TicksLeft: unitStats.TrainTicks,
		StartTick: w.Tick,
	})
}
