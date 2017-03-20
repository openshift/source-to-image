package api

import "time"

// RecordStageAndStepInfo records details about each build stage and step
func RecordStageAndStepInfo(stages Stages, stageName StageName, stepName StepName, startTime time.Time, endTime time.Time) Stages {
	// Make sure that the stages slice is initialized
	if len(stages) == 0 {
		stages = make([]StageInfo, 0)
	}

	// If the stage already exists  update the endTime and Duration, and append the new step.
	for stageKey, stageVal := range stages {
		if stageVal.StageName == stageName {
			stages[stageKey].Duration = endTime.Sub(stages[stageKey].StartTime)
			if len(stages[stageKey].Steps) == 0 {
				stages[stageKey].Steps = make([]StepInfo, 0)
			}
			stages[stageKey].Steps = append(stages[stageKey].Steps, StepInfo{
				StepName:  stepName,
				StartTime: startTime,
				Duration:  endTime.Sub(startTime),
			})
			return stages
		}
	}

	// If the stageName does not exist, add it to the slice along with the new step.
	steps := make([]StepInfo, 0)
	steps = append(steps, StepInfo{
		StepName:  stepName,
		StartTime: startTime,
		Duration:  endTime.Sub(startTime),
	})
	stages = append(stages, StageInfo{
		StageName: stageName,
		StartTime: startTime,
		Duration:  endTime.Sub(startTime),
		Steps:     steps,
	})
	return stages
}
