package reconcilertest

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func New[R reconcile.Reconciler, T any, PT interface {
	*T
	client.Object
}]() Scenario[R, T, PT] {
	return &scenario[R, T, PT]{}
}

type Scenario[R reconcile.Reconciler, T any, PT interface {
	*T
	client.Object
}] interface {
	WithSchemes(...func(s *runtime.Scheme) error) Scenario[R, T, PT]
	Setup(func() []client.Object) _reconcileStart[T, PT]
}

type _reconcileStart[T any, PT interface {
	*T
	client.Object
}] interface {
	_reconcileLoop[T, PT]
	StartReconcileFor(name string, namespace ...string) _reconcileLoop[T, PT]
}

type _reconcileLoop[T any, PT interface {
	*T
	client.Object
}] interface {
	_reconcileLeaf[T, PT]
	ReconcileUntil(f func(client client.Client, obj PT) bool, labels ...string) _reconcileAction[T, PT]
}

type _reconcileAction[T any, PT interface {
	*T
	client.Object
}] interface {
	_reconcileLoop[T, PT]
	Then(f func(t *testing.T, client client.Client, obj PT), labels ...string) _reconcileLoop[T, PT]
}

type _reconcileLeaf[T any, PT interface {
	*T
	client.Object
}] interface {
	Testable
	Case() Testable
}

type Testable interface {
	Test(t *testing.T)
}

// ---------------------------------

type reconcileHandler[PT runtime.Object] struct {
	waitFor      func(client client.Client, obj PT) bool
	waitForLabel string

	action      func(t *testing.T, client client.Client, obj PT)
	actionLabel string
}
type scenario[R reconcile.Reconciler, T any, PT interface {
	*T
	client.Object
}] struct {
	schemes      []func(*runtime.Scheme) error
	setupHandler func() []client.Object
	startObj     types.NamespacedName
	handlers     []reconcileHandler[PT]
}

func (s *scenario[R, T, PT]) WithSchemes(schemes ...func(s *runtime.Scheme) error) Scenario[R, T, PT] {
	s.schemes = append(s.schemes, schemes...)
	return s
}

func (s *scenario[R, T, PT]) Setup(sh func() []client.Object) _reconcileStart[T, PT] {
	s.setupHandler = sh
	return s
}

func (s *scenario[R, T, PT]) StartReconcileFor(name string, namespace ...string) _reconcileLoop[T, PT] {
	s.startObj.Name = name
	if len(namespace) > 0 {
		s.startObj.Namespace = namespace[0]
	}
	return s
}

func (s *scenario[R, T, PT]) ReconcileUntil(waitFor func(client client.Client, obj PT) bool, labels ...string) _reconcileAction[T, PT] {
	s.handlers = append(s.handlers, reconcileHandler[PT]{
		waitFor:      waitFor,
		waitForLabel: strings.Join(labels, ", "),
	})
	return s
}

func (s *scenario[R, T, PT]) Then(action func(t *testing.T, client client.Client, obj PT), labels ...string) _reconcileLoop[T, PT] {
	s.handlers[len(s.handlers)-1].action = action
	s.handlers[len(s.handlers)-1].actionLabel = strings.Join(labels, ", ")
	return s
}

func (s *scenario[R, T, PT]) Case() Testable {
	return s
}

func (s *scenario[R, T, PT]) Test(t *testing.T) {

	scheme := runtime.NewScheme()

	var err error
	for _, addToScheme := range s.schemes {
		if err = addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	objs := s.setupHandler()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(objs...).Build()

	reconciler := s.createReconcilerWithClient(fakeClient)

	s.setStartObj(t, objs)

	maxReconciles := 20
	stepIndex := 0
	loopCounter := 0

	nextStep := s.handlers[stepIndex]
	for {
		_, err = reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: s.startObj})
		if err != nil {
			assert.NoError(t, err)
			t.FailNow()
		}

		latestUpdatedObj := PT(new(T))
		err := fakeClient.Get(context.TODO(), s.startObj, latestUpdatedObj)

		if errors.IsNotFound(err) {
			latestUpdatedObj = nil
		} else {
			assert.NoError(t, err)
		}

		if nextStep.waitFor(fakeClient, latestUpdatedObj) {
			if nextStep.action != nil {
				nextStep.action(t, fakeClient, latestUpdatedObj)
			}

			if stepIndex == len(s.handlers)-1 {
				break
			}

			stepIndex++
			loopCounter = 0
			nextStep = s.handlers[stepIndex]
		}

		loopCounter++
		if loopCounter >= maxReconciles {
			label := s.handlers[stepIndex].waitForLabel
			if label == "" {
				label = fmt.Sprintf("waiting condition #%s", strconv.Itoa(stepIndex))
			}
			t.Fatalf("`%s` not satisfied, too many reconcile loops", label)
		}
	}
}

func (s *scenario[R, T, PT]) createReconcilerWithClient(client client.Client) reconcile.Reconciler {
	reconciler := new(R)

	fv := reflect.ValueOf(reconciler).Elem().FieldByName("Client")
	fv.Set(reflect.ValueOf(client))

	return *reconciler
}

func (s *scenario[R, T, PT]) setStartObj(t *testing.T, objs []client.Object) {

	// If no starting object is defined, let's pick up the first one
	// from the current cache that matches the configured type PT
	if s.startObj.Name == "" {

		for _, o := range objs {
			if _, ok := o.(PT); ok {
				accessor, err := meta.Accessor(o)
				assert.NoError(t, err)
				s.startObj = types.NamespacedName{
					Name:      accessor.GetName(),
					Namespace: accessor.GetNamespace(),
				}
				return
			}
		}

		assert.FailNow(t, "No eligible starting object found for the reconciler")
	}

	for _, o := range objs {
		accessor, err := meta.Accessor(o)
		assert.NoError(t, err)

		if s.startObj.Name == accessor.GetName() {
			// If no namespace was provided, let's default it
			// to the current match
			if s.startObj.Namespace == "" {
				s.startObj.Namespace = accessor.GetNamespace()
			}

			if s.startObj.Namespace == accessor.GetNamespace() {
				return
			}
		}
	}

	objId := s.startObj.Name
	if s.startObj.Namespace != "" {
		objId = fmt.Sprintf("%s:%s", s.startObj.Namespace, s.startObj.Name)
	}

	assert.FailNow(t, fmt.Sprintf("Object %s not found in the current cache", objId))
}
