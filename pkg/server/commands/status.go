package commands

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	"github.com/openshift/ofcir/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type statusCmd struct {
	context   *gin.Context
	clientset *ofcirclientv1.OfcirV1Client
	namespace string
	cirName   string
}

func NewStatusCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client, ns string, cirName string) command {
	return &statusCmd{
		context:   c,
		clientset: clientset,
		namespace: ns,
		cirName:   cirName,
	}
}

func (c *statusCmd) Run() error {
	overallCtx, overallCancel := context.WithTimeout(c.context.Request.Context(), overallTimeout)
	defer overallCancel()

	getCtx, getCancel := context.WithTimeout(overallCtx, apiCallTimeout)
	defer getCancel()

	r, err := c.clientset.CIResources(c.namespace).Get(getCtx, c.cirName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.context.JSON(http.StatusBadRequest, gin.H{
				"msg": fmt.Sprintf("%s does not exist in namespace %s", c.cirName, c.namespace),
			})
			return nil
		}
		return err
	}

	if !utils.CanUsePool(c.context, r.Spec.PoolRef.Name) {
		c.context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "401 Unauthorized"})
		return nil
	}

	poolCtx, poolCancel := context.WithTimeout(overallCtx, apiCallTimeout)
	defer poolCancel()

	pool, err := c.clientset.CIPools(c.namespace).Get(poolCtx, r.Spec.PoolRef.Name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.context.JSON(http.StatusBadRequest, gin.H{
				"msg": fmt.Sprintf("Cannot find cipool %s for %s in namespace %s", r.Spec.PoolRef.Name, c.cirName, c.namespace),
			})
			return nil
		}
		return err
	}

	c.context.JSON(http.StatusOK, gin.H{
		"name":         r.Name,
		"pool":         pool.Name,
		"provider":     pool.Spec.Provider,
		"providerInfo": r.Status.ProviderInfo,
		"type":         r.Spec.Type,
		"ip":           r.Status.Address,
		"extra":        r.Status.Extra,
		"status":       r.Status.State,
	})

	return nil
}
