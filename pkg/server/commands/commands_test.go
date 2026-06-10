package commands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	clientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Fakes ---

type fakeCIPoolClient struct {
	pools    *ofcirv1.CIPoolList
	pool     *ofcirv1.CIPool
	listErr  error
	getErr   error
	delay    time.Duration
	listHits int32
}

func (f *fakeCIPoolClient) List(ctx context.Context, _ metav1.ListOptions) (*ofcirv1.CIPoolList, error) {
	atomic.AddInt32(&f.listHits, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.pools, f.listErr
}

func (f *fakeCIPoolClient) Get(ctx context.Context, name string, _ metav1.GetOptions) (*ofcirv1.CIPool, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.pool, f.getErr
}

type fakeCIResourceClient struct {
	resources  *ofcirv1.CIResourceList
	resource   *ofcirv1.CIResource
	listErr    error
	getErr     error
	updateErr  error
	delay      time.Duration
	updateHits int32
}

func (f *fakeCIResourceClient) List(ctx context.Context, _ metav1.ListOptions) (*ofcirv1.CIResourceList, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resources, f.listErr
}

func (f *fakeCIResourceClient) Get(ctx context.Context, _ string, _ metav1.GetOptions) (*ofcirv1.CIResource, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resource, f.getErr
}

func (f *fakeCIResourceClient) Update(ctx context.Context, cir *ofcirv1.CIResource, _ metav1.UpdateOptions) (*ofcirv1.CIResource, error) {
	atomic.AddInt32(&f.updateHits, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return cir, f.updateErr
}

type fakeOfcirClient struct {
	poolClient     *fakeCIPoolClient
	resourceClient *fakeCIResourceClient
}

func (f *fakeOfcirClient) CIPools(_ string) clientv1.CIPoolInterface {
	return f.poolClient
}

func (f *fakeOfcirClient) CIResources(_ string) clientv1.CIResourceInterface {
	return f.resourceClient
}

// --- Helpers ---

func newTestGinContext(reqCtx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/ofcir", nil)
	if reqCtx != nil {
		req = req.WithContext(reqCtx)
	}
	c.Request = req
	c.Set("validpools", "*")
	return c, w
}

func makePool(name string, priority int, rtype ofcirv1.CIResourceType) ofcirv1.CIPool {
	return ofcirv1.CIPool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: ofcirv1.CIPoolSpec{
			Provider: "fake",
			Priority: priority,
			Type:     rtype,
		},
	}
}

func makeResource(name, poolName string, specState, statusState ofcirv1.CIResourceState) ofcirv1.CIResource {
	return ofcirv1.CIResource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: ofcirv1.CIResourceSpec{
			PoolRef: corev1.LocalObjectReference{Name: poolName},
			State:   specState,
			Type:    ofcirv1.TypeCIHost,
		},
		Status: ofcirv1.CIResourceStatus{
			State: statusState,
		},
	}
}

// --- Tests ---

func TestAcquireOverallContextDeadline(t *testing.T) {
	poolClient := &fakeCIPoolClient{
		delay: 200 * time.Millisecond,
		pools: &ofcirv1.CIPoolList{
			Items: []ofcirv1.CIPool{makePool("pool-1", 0, ofcirv1.TypeCIHost)},
		},
	}
	resourceClient := &fakeCIResourceClient{
		delay: 200 * time.Millisecond,
		resources: &ofcirv1.CIResourceList{
			Items: []ofcirv1.CIResource{
				makeResource("cir-0", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
			},
		},
	}
	client := &fakeOfcirClient{poolClient: poolClient, resourceClient: resourceClient}

	// Use a request context that expires before the fake delays complete
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer reqCancel()

	c, _ := newTestGinContext(reqCtx)
	cmd := NewAcquireCmd(c, client, "test-ns", string(ofcirv1.TypeCIHost))

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if !isContextError(err) {
		t.Fatalf("expected context error, got: %v", err)
	}
}

func TestLookForAvailableResourceBailsOnExpiredContext(t *testing.T) {
	resourceClient := &fakeCIResourceClient{}
	client := &fakeOfcirClient{
		poolClient:     &fakeCIPoolClient{},
		resourceClient: resourceClient,
	}

	c, _ := newTestGinContext(context.Background())
	acq := &acquireCmd{
		context:       c,
		clientset:     client,
		namespace:     "test-ns",
		resourceTypes: []ofcirv1.CIResourceType{ofcirv1.TypeCIHost},
	}

	cirs := []ofcirv1.CIResource{
		makeResource("cir-0", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
	}
	poolsByName := map[string]ofcirv1.CIPool{
		"pool-1": makePool("pool-1", 0, ofcirv1.TypeCIHost),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	result := acq.lookForAvailableResource(ctx, cirs, poolsByName)
	if result {
		t.Fatal("expected false when context is already cancelled")
	}
	if atomic.LoadInt32(&resourceClient.updateHits) != 0 {
		t.Fatal("expected no Update calls when context is cancelled")
	}
}

func TestAcquirePerCallTimeoutIsolation(t *testing.T) {
	poolClient := &fakeCIPoolClient{
		delay: 100 * time.Millisecond,
		pools: &ofcirv1.CIPoolList{
			Items: []ofcirv1.CIPool{makePool("pool-1", 0, ofcirv1.TypeCIHost)},
		},
	}
	resourceClient := &fakeCIResourceClient{
		delay: 100 * time.Millisecond,
		resources: &ofcirv1.CIResourceList{
			Items: []ofcirv1.CIResource{
				makeResource("cir-0", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
			},
		},
	}
	client := &fakeOfcirClient{poolClient: poolClient, resourceClient: resourceClient}

	// Overall budget is generous; each call is slow but within per-call limit
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	c, w := newTestGinContext(reqCtx)
	cmd := NewAcquireCmd(c, client, "test-ns", string(ofcirv1.TypeCIHost))

	err := cmd.Run()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&resourceClient.updateHits) != 1 {
		t.Fatal("expected exactly one Update call")
	}
}

func TestAcquireClientDisconnection(t *testing.T) {
	poolClient := &fakeCIPoolClient{
		delay: 500 * time.Millisecond,
		pools: &ofcirv1.CIPoolList{
			Items: []ofcirv1.CIPool{makePool("pool-1", 0, ofcirv1.TypeCIHost)},
		},
	}
	resourceClient := &fakeCIResourceClient{
		resources: &ofcirv1.CIResourceList{},
	}
	client := &fakeOfcirClient{poolClient: poolClient, resourceClient: resourceClient}

	// Simulate client disconnect: context cancelled before any work starts
	reqCtx, reqCancel := context.WithCancel(context.Background())
	reqCancel()

	c, _ := newTestGinContext(reqCtx)

	start := time.Now()
	cmd := NewAcquireCmd(c, client, "test-ns", string(ofcirv1.TypeCIHost))

	err := cmd.Run()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled request context, got nil")
	}
	if !isContextError(err) {
		t.Fatalf("expected context error, got: %v", err)
	}
	// Should return nearly instantly, not wait for the 500ms fake delay
	if elapsed > 100*time.Millisecond {
		t.Fatalf("expected fast return on cancelled context, took %v", elapsed)
	}
	if atomic.LoadInt32(&resourceClient.updateHits) != 0 {
		t.Fatal("expected no Update calls when client is disconnected")
	}
}

func isContextError(err error) bool {
	return err == context.DeadlineExceeded || err == context.Canceled
}

func TestConcurrentAcquireDoesNotStarve(t *testing.T) {
	poolClient := &fakeCIPoolClient{
		delay: 50 * time.Millisecond,
		pools: &ofcirv1.CIPoolList{
			Items: []ofcirv1.CIPool{makePool("pool-1", 0, ofcirv1.TypeCIHost)},
		},
	}
	resourceClient := &fakeCIResourceClient{
		delay: 50 * time.Millisecond,
		resources: &ofcirv1.CIResourceList{
			Items: []ofcirv1.CIResource{
				makeResource("cir-0", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
				makeResource("cir-1", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
				makeResource("cir-2", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
				makeResource("cir-3", "pool-1", ofcirv1.StateAvailable, ofcirv1.StateAvailable),
			},
		},
	}
	client := &fakeOfcirClient{poolClient: poolClient, resourceClient: resourceClient}

	const numRequests = 20
	const maxAllowed = 2 * time.Second

	type result struct {
		err      error
		code     int
		duration time.Duration
	}
	results := make([]result, numRequests)
	var wg sync.WaitGroup

	for i := range numRequests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c, w := newTestGinContext(reqCtx)
			cmd := NewAcquireCmd(c, client, "test-ns", string(ofcirv1.TypeCIHost))

			start := time.Now()
			err := cmd.Run()
			results[idx] = result{
				err:      err,
				code:     w.Code,
				duration: time.Since(start),
			}
		}(i)
	}
	wg.Wait()

	var maxDur time.Duration
	var errors int
	for i, res := range results {
		if res.duration > maxDur {
			maxDur = res.duration
		}
		if res.err != nil {
			errors++
			t.Logf("request %02d: err=%v duration=%v", i, res.err, res.duration)
		}
	}

	if maxDur > maxAllowed {
		t.Fatalf("slowest request took %v, expected under %v (possible rate limiting starvation)", maxDur, maxAllowed)
	}
	if errors > 0 {
		t.Fatalf("%d/%d requests failed unexpectedly", errors, numRequests)
	}
	t.Logf("all %d concurrent requests completed, max duration: %v", numRequests, maxDur)
}
