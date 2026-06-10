package commands

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	"github.com/openshift/ofcir/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type releaseCmd struct {
	context   *gin.Context
	clientset *ofcirclientv1.OfcirV1Client
	namespace string
	cirName   string
}

func NewReleaseCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client, ns string, cirName string) command {
	return &releaseCmd{
		context:   c,
		clientset: clientset,
		namespace: ns,
		cirName:   cirName,
	}
}

func (c *releaseCmd) Run() error {
	overallCtx, overallCancel := context.WithTimeout(c.context.Request.Context(), 55*time.Second)
	defer overallCancel()

	getCtx, getCancel := context.WithTimeout(overallCtx, 18*time.Second)
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

	switch r.Status.State {
	case ofcirv1.StateInUse:
		r.Spec.State = ofcirv1.StateAvailable
		updateCtx, updateCancel := context.WithTimeout(overallCtx, 18*time.Second)
		defer updateCancel()
		_, err := c.clientset.CIResources(r.Namespace).Update(updateCtx, r, v1.UpdateOptions{})
		if err != nil {
			return err
		}

		c.context.String(http.StatusOK, r.Name)

	default:
		c.context.JSON(http.StatusBadRequest, gin.H{
			"msg": fmt.Sprintf("%s state must be `%s`, but it is `%s`", c.cirName, ofcirv1.StateInUse, r.Status.State),
		})
	}

	return nil
}
