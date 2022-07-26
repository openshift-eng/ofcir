package commands

import (
	"context"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type acquireCmd struct {
	context   *gin.Context
	clientset *ofcirclientv1.OfcirV1Client
}

func NewAcquireCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client) command {
	return &acquireCmd{
		context:   c,
		clientset: clientset,
	}
}

func (c *acquireCmd) Run() error {

	pools, err := c.clientset.CIPools("").List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	poolsByName := make(map[string]ofcirv1.CIPool)
	for _, p := range pools.Items {
		poolsByName[p.Name] = p
	}

	allCirs, err := c.clientset.CIResources("").List(context.Background(), v1.ListOptions{})
	if err != nil {
		return err
	}

	var cirs, fallbacks []ofcirv1.CIResource

	for _, r := range allCirs.Items {
		pool := poolsByName[r.Spec.PoolRef.Name]
		if pool.Spec.Priority < 0 {
			fallbacks = append(fallbacks, r)
		} else {
			cirs = append(cirs, r)
		}
	}

	sort.SliceStable(cirs, func(i, j int) bool {
		cir0 := cirs[i]
		pool0 := poolsByName[cir0.Spec.PoolRef.Name]

		cir1 := cirs[j]
		pool1 := poolsByName[cir1.Spec.PoolRef.Name]

		return pool0.Spec.Priority < pool1.Spec.Priority
	})

	// Let's try to look for an available resource in the default pools
	if c.lookForAvailableResource(cirs) {
		return nil
	}

	// Let's try on the fallback one
	if c.lookForAvailableResource(fallbacks) {
		return nil
	}

	c.context.String(http.StatusNotFound, "No available resource found")
	return nil
}

func (c *acquireCmd) lookForAvailableResource(cirs []ofcirv1.CIResource) bool {
	for _, r := range cirs {
		if r.Status.State == ofcirv1.StateAvailable && r.Spec.State != ofcirv1.StateInUse {

			r.Spec.State = ofcirv1.StateInUse
			_, err := c.clientset.CIResources(r.Namespace).Update(context.Background(), &r, v1.UpdateOptions{})
			if err != nil {
				continue
			}

			c.context.String(http.StatusOK, r.Name)
			return true
		}
	}

	return false
}
