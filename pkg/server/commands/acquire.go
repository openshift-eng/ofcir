package commands

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	"github.com/openshift/ofcir/pkg/utils"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type acquireCmd struct {
	context       *gin.Context
	clientset     *ofcirclientv1.OfcirV1Client
	namespace     string
	resourceTypes []ofcirv1.CIResourceType
}

func NewAcquireCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client, ns string, resourceType_str string) command {
	// type can be a comma seperated list
	resourceTypes_split := strings.Split(resourceType_str, ",")
	resourceTypes := make([]ofcirv1.CIResourceType, len(resourceTypes_split))
	for i, v := range resourceTypes_split {
		resourceTypes[i] = ofcirv1.CIResourceType(v)
	}

	return &acquireCmd{
		context:       c,
		clientset:     clientset,
		namespace:     ns,
		resourceTypes: resourceTypes,
	}
}

func contains(rtypes []ofcirv1.CIResourceType, rtype ofcirv1.CIResourceType) bool {
	for _, t := range rtypes {
		if t == rtype {
			return true
		}
	}
	return false
}

func (c *acquireCmd) Run() error {

	pools, err := c.clientset.CIPools(c.namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return err
	}

	poolsByName := make(map[string]ofcirv1.CIPool)
	// c.resourceTypes is a list of cir types, no preference is given to the order
	for _, p := range pools.Items {
		if (contains(c.resourceTypes, p.Spec.Type)) && utils.CanUsePool(c.context, p.Name) {
			poolsByName[p.Name] = p
		}
	}

	if len(poolsByName) == 0 {
		c.context.String(http.StatusNotFound, fmt.Sprintf("No available pool found of type %v", c.resourceTypes))
		return nil
	}

	allCirs, err := c.clientset.CIResources(c.namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return err
	}

	var cirs, fallbacks []ofcirv1.CIResource

	for _, r := range allCirs.Items {
		pool, ok := poolsByName[r.Spec.PoolRef.Name]
		// This cir belongs to a filtered pool, let's skip it
		if !ok {
			continue
		}

		if pool.Spec.Priority < 0 {
			fallbacks = append(fallbacks, r)
		} else {
			cirs = append(cirs, r)
		}
	}

	rand.Shuffle(len(cirs), func(i, j int) { cirs[i], cirs[j] = cirs[j], cirs[i] })

	sort.SliceStable(cirs, func(i, j int) bool {
		cir0 := cirs[i]
		pool0 := poolsByName[cir0.Spec.PoolRef.Name]

		cir1 := cirs[j]
		pool1 := poolsByName[cir1.Spec.PoolRef.Name]

		return pool0.Spec.Priority < pool1.Spec.Priority
	})

	// Let's try to look for an available resource in the default pools
	if c.lookForAvailableResource(cirs, poolsByName) {
		return nil
	}

	// Let's try on the fallback one
	if c.lookForAvailableResource(fallbacks, poolsByName) {
		return nil
	}

	c.context.String(http.StatusNotFound, "No available resource found")
	return nil
}

func (c *acquireCmd) lookForAvailableResource(cirs []ofcirv1.CIResource, poolsByName map[string]ofcirv1.CIPool) bool {
	for _, r := range cirs {

		// Only available resource are eligible to be acquired
		if r.Status.State != ofcirv1.StateAvailable {
			continue
		}

		// Check if the resource is not being requested by someone else
		if r.Spec.State != ofcirv1.StateInUse && r.Spec.State != ofcirv1.StateMaintenance {

			r.Spec.State = ofcirv1.StateInUse
			_, err := c.clientset.CIResources(r.Namespace).Update(context.Background(), &r, v1.UpdateOptions{})
			if err != nil {
				continue
			}

			pool := poolsByName[r.Spec.PoolRef.Name]

			c.context.JSON(http.StatusOK, gin.H{
				"name":         r.Name,
				"pool":         pool.Name,
				"provider":     pool.Spec.Provider,
				"providerInfo": r.Status.ProviderInfo,
				"type":         r.Spec.Type,
			})
			return true
		}
	}

	return false
}
