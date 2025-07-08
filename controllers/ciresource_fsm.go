package controllers

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	fallbackResourceID              = "000-fallback-dummy-000" // dummy ID for fallback resource
	defaultCirRetryDelay            = time.Minute * 1
	defaultCirProvisioningWaitDelay = time.Second * 30
)

func NewCIResourceFSM(logger logr.Logger) *CIResourceFSM {
	fsm := &CIResourceFSM{
		states:      make(map[ofcirv1.CIResourceState]fsmState),
		logger:      logger,
		debuglogger: logger.V(1),
	}

	fsm.State(ofcirv1.StateNone,
		fsm.handleStateNone,
		Transition("init", ofcirv1.StateProvisioning))

	fsm.State(ofcirv1.StateProvisioning,
		fsm.handleStateProvisioning,
		Transition("on-provisioning-requested", ofcirv1.StateProvisioningWait),
		Transition("fallback-available", ofcirv1.StateAvailable))

	fsm.State(ofcirv1.StateProvisioningWait,
		fsm.handleStateProvisioningWait,
		Transition("on-provisioning-complete", ofcirv1.StateAvailable))

	fsm.State(ofcirv1.StateAvailable,
		fsm.handleStateAvailable,
		Transition("on-maintenance", ofcirv1.StateMaintenance),
		Transition("acquired", ofcirv1.StateInUse),
		Transition("on-delete", ofcirv1.StateDelete))

	fsm.State(ofcirv1.StateMaintenance,
		fsm.handleStateMaintenance,
		Transition("on-maintenance-complete", ofcirv1.StateAvailable),
		Transition("on-delete", ofcirv1.StateDelete))

	fsm.State(ofcirv1.StateInUse,
		fsm.handleStateInUse,
		Transition("released", ofcirv1.StateCleaning),
		Transition("fallback-provisioning", ofcirv1.StateProvisioning))

	fsm.State(ofcirv1.StateCleaning,
		fsm.handleStateCleaning,
		Transition("on-cleaning-requested", ofcirv1.StateCleaningWait))

	fsm.State(ofcirv1.StateCleaningWait,
		fsm.handleStateCleaningWait,
		Transition("on-cleaning-complete", ofcirv1.StateAvailable))

	fsm.State(ofcirv1.StateDelete,
		fsm.handleStateDelete)

	return fsm
}

func (f *CIResourceFSM) handleStateNone(context CIResourceFSMContext) (time.Duration, error) {
	// Check if cir contains a finalizer when is not under deletion
	if context.CIResource.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(context.CIResource, ofcirv1.OfcirFinalizer) {
			f.logger.Info("Adding finalizer")
			controllerutil.AddFinalizer(context.CIResource, ofcirv1.OfcirFinalizer)
			return f.UpdateResourceOnly()
		}
	}

	return f.TriggerEvent("init")
}

func (f *CIResourceFSM) handleStateProvisioning(context CIResourceFSMContext) (time.Duration, error) {

	// If a fallback resource is not requested then let's move it directly to the available state,
	// otherwise a normal provisioning phase is kicked off
	if context.CIPool.IsFallbackPool() && context.CIResource.Spec.State != ofcirv1.StateInUse {
		context.CIResource.Status.ResourceId = fallbackResourceID
		return f.TriggerEvent("fallback-available")
	}

	resource, err := context.Provider.Acquire(context.CIPool.Spec.Size, context.CIPool.Name, string(context.CIPool.Spec.Type))
	if err != nil {
		return 0, err
	}

	context.CIResource.Status.ResourceId = resource.Id
	context.CIResource.Status.ProviderInfo = context.CIPool.Spec.ProviderInfo
	f.logger.Info("provisioning new resource", "Id", resource.Id)

	return f.TriggerEvent("on-provisioning-requested")
}

func (f *CIResourceFSM) handleStateProvisioningWait(context CIResourceFSMContext) (time.Duration, error) {

	isReady, resource, err := context.Provider.AcquireCompleted(context.CIResource.Status.ResourceId)
	if err != nil {
		return 0, err
	}

	if isReady {
		context.CIResource.Status.Address = resource.Address
		context.CIResource.Status.Extra = resource.Metadata

		f.logger.Info("resource was provisioned", "Id", context.CIResource.Status.ResourceId, "Address", context.CIResource.Status.Address)
		f.TriggerEvent("on-provisioning-complete")
		return 0, nil
	}

	f.logger.Info("waiting for new resource to be provisioned", "Id", context.CIResource.Status.ResourceId)
	return defaultCirProvisioningWaitDelay, nil
}

func (f *CIResourceFSM) handleStateAvailable(context CIResourceFSMContext) (time.Duration, error) {

	//check for deletion
	if !context.CIResource.ObjectMeta.DeletionTimestamp.IsZero() {
		return f.TriggerEvent("on-delete")
	}

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

	//check for deletion
	if !context.CIResource.ObjectMeta.DeletionTimestamp.IsZero() {
		return f.TriggerEvent("on-delete")
	}

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

	switch context.CIResource.Spec.State {
	case ofcirv1.StateAvailable:
		return f.TriggerEvent("released")
	case ofcirv1.StateInUse:
		// A fallback resource has been requested, so it must be provisioned
		if context.CIPool.IsFallbackPool() && context.CIResource.Status.Address == "" && context.CIResource.Status.ResourceId == fallbackResourceID {
			return f.TriggerEvent("fallback-provisioning")
		}
		// CIR's can only be held "inuse" for a limited amount of time
		if time.Since(context.CIResource.Status.LastUpdated.Time) > context.CIPool.Spec.Timeout.Duration {
			f.logger.Info("releasing resource, max duration hit", "Id", context.CIResource.Status.ResourceId)
			context.CIResource.Spec.State = ofcirv1.StateAvailable
			return f.UpdateResourceOnly()
		}
	}

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) handleStateCleaning(context CIResourceFSMContext) (time.Duration, error) {

	// If it's a fallback resource, let's clean and release it immediately
	if context.CIPool.IsFallbackPool() {

		// Do not delete a fallback resource that was not yet created
		if context.CIResource.Status.ResourceId != fallbackResourceID {
			err := context.Provider.Release(context.CIResource.Status.ResourceId)
			if err != nil && !errors.As(err, &providers.ResourceNotFoundError{}) {
				return defaultCIPoolRetryDelay, err
			}
		}

		context.CIResource.Status.Address = ""
		context.CIResource.Status.Extra = ""
		context.CIResource.Status.ProviderInfo = ""
		context.CIResource.Status.ResourceId = fallbackResourceID
	} else if err := context.Provider.Clean(context.CIResource.Status.ResourceId); err != nil {
		return defaultCIPoolRetryDelay, err
	}
	return f.TriggerEvent("on-cleaning-requested")
}

func (f *CIResourceFSM) handleStateCleaningWait(context CIResourceFSMContext) (time.Duration, error) {

	if context.CIPool.IsFallbackPool() {
		return f.TriggerEvent("on-cleaning-complete")
	}

	isCleaned, err := context.Provider.CleanCompleted(context.CIResource.Status.ResourceId)
	if err != nil {
		return defaultCIPoolRetryDelay, err
	}

	if isCleaned {
		f.logger.Info("resource was cleaned", "Id", context.CIResource.Status.ResourceId, "Address", context.CIResource.Status.Address)
		return f.TriggerEvent("on-cleaning-complete")
	}

	f.logger.Info("waiting for resource to be cleaned", "Id", context.CIResource.Status.ResourceId)
	return defaultCirProvisioningWaitDelay, nil
}

func (f *CIResourceFSM) handleStateDelete(context CIResourceFSMContext) (time.Duration, error) {

	f.logger.Info("removing resource", "Id", context.CIResource.Status.ResourceId)

	if controllerutil.ContainsFinalizer(context.CIResource, ofcirv1.OfcirFinalizer) {

		// don't call the provider if the resource is a fallback dummy
		if context.CIPool.IsFallbackPool() && context.CIResource.Status.ResourceId == fallbackResourceID {
			controllerutil.RemoveFinalizer(context.CIResource, ofcirv1.OfcirFinalizer)
			return f.UpdateResourceOnly()
		}

		if err := context.Provider.Release(context.CIResource.Status.ResourceId); err != nil {
			if !errors.As(err, &providers.ResourceNotFoundError{}) {
				return defaultCIPoolRetryDelay, err
			}
		}

		controllerutil.RemoveFinalizer(context.CIResource, ofcirv1.OfcirFinalizer)
		return f.UpdateResourceOnly()
	}

	//no update
	return defaultCIPoolRetryDelay, nil
}

// ----------------------------------------------------------------------------

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
	logger      logr.Logger
	debuglogger logr.Logger

	currentState   *fsmState
	currentContext CIResourceFSMContext
	statusDirty    bool
	resourceDirty  bool
	states         map[ofcirv1.CIResourceState]fsmState
	beforeAnyState CIResourceFSMHandler
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
func (f *CIResourceFSM) BeforeAnyState(before CIResourceFSMHandler) {
	f.beforeAnyState = before
}

func (f *CIResourceFSM) Process(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool, cipoolSecret *v1.Secret) (bool, bool, time.Duration, error) {

	provider, err := providers.NewProvider(cipool, cipoolSecret, f.logger)
	if err != nil {
		return false, false, time.Duration(0), fmt.Errorf("error in provider factory: %w", err)
	}

	context := CIResourceFSMContext{
		CIResource: cir,
		CIPool:     cipool,
		Provider:   provider,
	}

	state, ok := f.states[context.CIResource.Status.State]
	if !ok {
		return false, false, time.Duration(0), fmt.Errorf("state not found: %s", context.CIResource.Status.State)
	}
	f.currentState = &state
	f.currentContext = context

	// Evaluate main handler before managing state ones
	if f.beforeAnyState != nil {
		retryAfter, err := f.beforeAnyState(context)
		if retryAfter != 0 || err != nil {
			return f.resourceDirty, f.statusDirty, retryAfter, fmt.Errorf("error in main handler: %w", err)
		}
	}

	f.debuglogger.Info("state -->", "state", state.id)
	retryAfter, err := state.onEntry(context)

	if err != nil {
		f.logger.Error(err, "error caught while processing state", "state", state.id)
	}
	f.debuglogger.Info("state <--", "state", state.id)

	return f.resourceDirty, f.statusDirty, retryAfter, err
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

	f.logger.Info("triggering state change", "id", f.currentContext.CIResource.Status.ResourceId, "current", f.currentContext.CIResource.Status.State, "new", t.dst)

	f.currentContext.CIResource.Status.State = t.dst
	f.statusDirty = true

	return defaultCirRetryDelay, nil
}

func (f *CIResourceFSM) UpdateResourceOnly() (time.Duration, error) {
	f.resourceDirty = true

	return defaultCirRetryDelay, nil
}
