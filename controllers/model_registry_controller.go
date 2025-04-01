package controllers

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	v1 "github.com/neutree-ai/neutree/api/v1"
	"github.com/neutree-ai/neutree/pkg/model_registry"
	"github.com/neutree-ai/neutree/pkg/storage"
)

type ModelRegistryController struct {
	storage *storage.Storage

	baseController *BaseController
}

type ModelRegistryControllerOption struct {
	*storage.Storage
	Workers int
}

func NewModelRegistryController(option *ModelRegistryControllerOption) (*ModelRegistryController, error) {
	c := &ModelRegistryController{
		baseController: &BaseController{
			queue:        workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: "model-registry"}),
			workers:      option.Workers,
			syncInterval: time.Second * 10,
		},
		storage: option.Storage,
	}

	return c, nil
}

func (c *ModelRegistryController) Start(ctx context.Context) {
	klog.Infof("Starting model registry controller")

	c.baseController.Start(ctx, c)
}

func (c *ModelRegistryController) Sync(obj interface{}) error {
	return c.sync(obj.(*v1.ModelRegistry))
}

func (c *ModelRegistryController) ListObjects() ([]interface{}, error) {
	registries, err := c.storage.ListModelRegistry(storage.ListOption{})
	if err != nil {
		return nil, err
	}

	objs := make([]interface{}, len(registries))
	for i := range registries {
		objs[i] = &registries[i]
	}
	return objs, nil
}

func (c *ModelRegistryController) ProcessObject(obj interface{}) (interface{}, error) {
	registry, ok := obj.(*v1.ModelRegistry)
	if !ok {
		return nil, errors.New("failed to assert obj to ModelRegistry")
	}
	return registry, nil
}

func (c *ModelRegistryController) sync(obj *v1.ModelRegistry) (err error) {
	modelRegistry, err := model_registry.New(obj)
	if err != nil {
		return err
	}

	if obj.Metadata.DeletionTimestamp != "" {
		if obj.Status.Phase == v1.ModelRegistryPhaseDELETED {
			klog.Info("Deleted model registry " + obj.Metadata.Name)

			err = c.storage.DeleteModelRegistry(strconv.Itoa(obj.ID))
			if err != nil {
				return errors.Wrap(err, "failed to delete model registry "+obj.Metadata.Name)
			}

			return nil
		}

		klog.Info("Deleting model registry " + obj.Metadata.Name)

		if err = modelRegistry.Disconnect(); err != nil {
			return errors.Wrap(err, "failed to disconnect model registry "+obj.Metadata.Name)
		}

		if err = c.updateStatus(obj, v1.ModelRegistryPhaseDELETED, nil); err != nil {
			return errors.Wrap(err, "failed to update model registry "+obj.Metadata.Name)
		}

		return nil
	}

	defer func() {
		phase := v1.ModelRegistryPhaseCONNECTED
		if err != nil {
			phase = v1.ModelRegistryPhaseFAILED
		}

		if obj.Status.Phase == phase {
			return
		}

		updateStatusErr := c.updateStatus(obj, phase, err)
		if updateStatusErr != nil {
			klog.Error(updateStatusErr, "failed to update model registry status")
		}
	}()

	if obj.Status.Phase == "" || obj.Status.Phase == v1.ModelRegistryPhasePENDING {
		klog.Info("Connect model registry " + obj.Metadata.Name)

		err = modelRegistry.Connect()
		if err != nil {
			return errors.Wrap(err, "failed to connect model registry "+obj.Metadata.Name)
		}

		return nil
	}

	if obj.Status.Phase == v1.ModelRegistryPhaseFAILED {
		klog.Info("Reconnect model registry " + obj.Metadata.Name)

		if err = modelRegistry.Disconnect(); err != nil {
			return errors.Wrap(err, "failed to disconnect model registry "+obj.Metadata.Name)
		}

		if err = modelRegistry.Connect(); err != nil {
			return errors.Wrap(err, "failed to connect model registry "+obj.Metadata.Name)
		}

		return nil
	}

	if obj.Status.Phase == v1.ModelRegistryPhaseCONNECTED {
		klog.Info("Health check model registry " + obj.Metadata.Name)

		healthy := modelRegistry.HealthyCheck()
		if !healthy {
			return errors.New("health check failed")
		}
	}

	return nil
}

func (c *ModelRegistryController) updateStatus(obj *v1.ModelRegistry, phase v1.ModelRegistryPhase, err error) error {
	obj.Status = v1.ModelRegistryStatus{
		LastTransitionTime: time.Now().Format(time.RFC3339Nano),
		Phase:              phase,
	}
	if err != nil {
		obj.Status.ErrorMessage = err.Error()
	}

	updateStatusErr := c.storage.UpdateModelRegistry(strconv.Itoa(obj.ID), obj)
	if err != nil {
		return errors.Wrap(updateStatusErr, "failed to update model registry "+obj.Metadata.Name)
	}

	return updateStatusErr
}
