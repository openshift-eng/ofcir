package controllers

import (
	"fmt"
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
)

const (
	defaultCirRetryDelay = time.Minute * 5
)

func NewCIResourceFSM() *CIResourceFSM {
	fsm := &CIResourceFSM{
		states: make(map[ofcirv1.CIResourceState]fsmState),
	}

	fsm.State(ofcirv1.StateNone,
		fsm.handleStateNone,
		Transition("init", ofcirv1.StateProvisioning))

	fsm.State(ofcirv1.StateProvisioning,
		fsm.handleStateProvisioning,
		Transition("on-provisioning-complete", ofcirv1.StateAvailable),
		Transition("on-provisioning-error", ofcirv1.StateError))

	fsm.State(ofcirv1.StateAvailable,
		fsm.handleStateAvailable,
		Transition("on-maintenance", ofcirv1.StateMaintenance),
		Transition("acquired", ofcirv1.StateInUse))

	fsm.State(ofcirv1.StateMaintenance,
		fsm.handleStateMaintenance,
		Transition("on-maintenance-complete", ofcirv1.StateAvailable))

	fsm.State(ofcirv1.StateInUse,
		fsm.handleStateInUse,
		Transition("released", ofcirv1.StateCleaning))

	fsm.State(ofcirv1.StateCleaning,
		fsm.handleStateCleaning,
		Transition("on-cleaning-complete", ofcirv1.StateAvailable),
		Transition("on-cleaning-error", ofcirv1.StateError))

	fsm.State(ofcirv1.StateError,
		fsm.handleStateError)

	fsm.BeforeAnyState(func(context CIResourceFSMContext) bool {
		labels := context.CIResource.GetLabels()
		if labels == nil {
			return true
		}

		if _, found := labels[ofcirv1.EvictionLabel]; found {
			return false
		}

		return true
	})

	return fsm
}

func (f *CIResourceFSM) handleStateNone(context CIResourceFSMContext) (time.Duration, error) {
	return f.TriggerEvent("init")
}

func (f *CIResourceFSM) handleStateProvisioning(context CIResourceFSMContext) (time.Duration, error) {

	//TODO: Based on pool provider type, provision a new resorce
	f.TriggerEvent("on-provisioning-complete")

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateAvailable(context CIResourceFSMContext) (time.Duration, error) {

	if context.CIResource.Spec.State == context.CIResource.Status.State {
		return defaultCirRetryDelay, nil
	}

	switch context.CIResource.Spec.State {
	case ofcirv1.StateMaintenance:
		return f.TriggerEvent("on-maintenance")
	case ofcirv1.StateInUse:
		return f.TriggerEvent("acquired")
	}

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateMaintenance(context CIResourceFSMContext) (time.Duration, error) {

	if context.CIResource.Spec.State == context.CIResource.Status.State {
		return defaultCirRetryDelay, nil
	}

	switch context.CIResource.Spec.State {
	case ofcirv1.StateAvailable:
		return f.TriggerEvent("on-maintenance-complete")
	}

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateInUse(context CIResourceFSMContext) (time.Duration, error) {

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateCleaning(context CIResourceFSMContext) (time.Duration, error) {

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateError(context CIResourceFSMContext) (time.Duration, error) {

	return defaultCirRetryDelay, nil
}

// CIResourceFSMHandler is used to handle a state process
type CIResourceFSMHandler func(context CIResourceFSMContext) (time.Duration, error)

// CIResourceFSMGuard is used when moving from one state to another one
type CIResourceFSMGuard func(context CIResourceFSMContext) bool

type fsmTransition struct {
	eventId string
	dst     ofcirv1.CIResourceState
	guard   CIResourceFSMGuard
}

// Triggers the transition from one state to another one. A guard condition could be specified
// to allow/deny the transition
func Transition(eventId string, dst ofcirv1.CIResourceState, guard ...CIResourceFSMGuard) *fsmTransition {
	t := &fsmTransition{
		eventId: eventId,
		dst:     dst,
	}

	if len(guard) == 1 {
		t.guard = guard[0]
	}

	return t
}

type fsmState struct {
	id          ofcirv1.CIResourceState
	transitions map[string]fsmTransition
	onEntry     CIResourceFSMHandler
}

type CIResourceFSMContext struct {
	CIResource *ofcirv1.CIResource
	CIPool     *ofcirv1.CIPool
	Provider   providers.Provider
}

type CIResourceFSM struct {
	currentState   *fsmState
	currentContext CIResourceFSMContext
	dirty          bool
	states         map[ofcirv1.CIResourceState]fsmState
	beforeState    CIResourceFSMGuard
}

func (f *CIResourceFSM) State(id ofcirv1.CIResourceState, onEntry CIResourceFSMHandler, transitions ...*fsmTransition) *CIResourceFSM {
	state := fsmState{
		id:          id,
		onEntry:     onEntry,
		transitions: make(map[string]fsmTransition),
	}

	for _, t := range transitions {
		state.transitions[t.eventId] = *t
	}
	f.states[id] = state

	return f
}

// This handler will be invoked before evaluating any selected state
func (f *CIResourceFSM) BeforeAnyState(before CIResourceFSMGuard) {
	f.beforeState = before
}

func (f *CIResourceFSM) Process(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) (bool, time.Duration, error) {

	provider, err := providers.NewProvider(cipool.Spec.Provider)
	if err != nil {
		return false, time.Duration(0), err
	}

	context := CIResourceFSMContext{
		CIResource: cir,
		CIPool:     cipool,
		Provider:   provider,
	}

	state, ok := f.states[context.CIResource.Status.State]
	if !ok {
		return false, time.Duration(0), fmt.Errorf("State not found: %s", context.CIResource.Spec.State)
	}
	f.currentState = &state
	f.currentContext = context

	if f.beforeState != nil && !f.beforeState(context) {
		return false, time.Duration(0), nil
	}
	retryAfter, err := state.onEntry(context)

	return f.dirty, retryAfter, err
}

func (f *CIResourceFSM) TriggerEvent(name string) (time.Duration, error) {

	if f.currentState == nil {
		return time.Duration(0), fmt.Errorf("no state selected")
	}

	t, ok := f.currentState.transitions[name]
	if !ok {
		return time.Duration(0), fmt.Errorf("event not found: %s", name)
	}

	if t.guard != nil && !t.guard(f.currentContext) {
		return defaultCirRetryDelay, nil
	}

	f.currentContext.CIResource.Status.State = t.dst
	f.dirty = true

	return defaultCirRetryDelay, nil
}
