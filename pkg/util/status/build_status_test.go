package status

import (
	"testing"
	"time"

	"github.com/openshift/source-to-image/pkg/api"
)

func TestNewFailureReason(t *testing.T) {

	failureReason := NewFailureReason(ReasonAssembleFailed, ReasonMessageAssembleFailed)

	if failureReason.Reason != ReasonAssembleFailed {
		t.Errorf("Expected reason to be: %s, got %s", ReasonAssembleFailed, failureReason.Reason)
	}

	if failureReason.Message != ReasonMessageAssembleFailed {
		t.Errorf("Expected message reason to be: %s, got %s", ReasonMessageAssembleFailed, failureReason.Message)
	}
}

func TestAddStepInfoNewStep(t *testing.T) {
	stepInfo := []api.BuildStepInfo{
		{
			Name: api.GatherInputContentStep,
		},
	}

	result := AddStepInfo(stepInfo, api.AssembleScriptStep, time.Now())
	for _, i := range result {
		if i.Name != api.AssembleScriptStep {
			continue
		}
		return
	}
	t.Errorf("%s step info array not updated, got %#v", stepInfo, result)
}

func TestAddStepInfoModStep(t *testing.T) {
	// the expected value of the test is for the StopTime of the step to be
	// different than zero.
	minute := time.Now().Minute()

	stepInfo := []api.BuildStepInfo{
		{
			Name: api.GatherInputContentStep,
		},
	}
	result := AddStepInfo(stepInfo, api.GatherInputContentStep, time.Now())

	for _, step := range result {
		if step.Name != api.GatherInputContentStep {
			continue
		}
		if step.StopTime.Minute() != minute {
			t.Errorf("Expected step time to be update to: %#v, got: %#v", minute, step.StopTime.Minute())
		}
	}
}
