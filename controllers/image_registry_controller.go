package controllers

import (
	"context"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	v1 "github.com/neutree-ai/neutree/api/v1"
	"github.com/neutree-ai/neutree/pkg/storage"
)

type ImageRegistryController struct {
	storage *storage.Storage

	dockerClient *client.Client

	baseController *BaseController
}

type ImageRegistryControllerOption struct {
	*storage.Storage
	Workers int
}

func NewImageRegistryController(option *ImageRegistryControllerOption) (*ImageRegistryController, error) {
	c := &ImageRegistryController{
		baseController: &BaseController{
			queue:        workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(), workqueue.RateLimitingQueueConfig{Name: "image-registry"}),
			workers:      option.Workers,
			syncInterval: time.Second * 10,
		},
		storage: option.Storage,
	}

	var err error
	// todo
	// depend on docker daemon
	c.dockerClient, err = client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Docker client")
	}

	return c, nil
}

func (c *ImageRegistryController) Start(ctx context.Context) {
	klog.Infof("Starting image registry controller")

	c.baseController.Start(ctx, c)
}

func (c *ImageRegistryController) Sync(obj interface{}) error {
	return c.sync(obj.(*v1.ImageRegistry))
}

func (c *ImageRegistryController) ListObjects() ([]interface{}, error) {
	registries, err := c.storage.ListImageRegistry(storage.ListOption{})
	if err != nil {
		return nil, err
	}

	objs := make([]interface{}, len(registries))
	for i := range registries {
		objs[i] = &registries[i]
	}
	return objs, nil
}

func (c *ImageRegistryController) ProcessObject(obj interface{}) (interface{}, error) {
	registry, ok := obj.(*v1.ImageRegistry)
	if !ok {
		return nil, errors.New("failed to assert obj to ImageRegistry")
	}
	return registry, nil
}

func (c *ImageRegistryController) sync(obj *v1.ImageRegistry) error {
	var err error

	if obj.Metadata.DeletionTimestamp != "" {
		if obj.Status.Phase == v1.ImageRegistryPhaseDELETED {
			klog.Info("Deleted image registry " + obj.Metadata.Name)

			err = c.storage.DeleteImageRegistry(strconv.Itoa(obj.ID))
			if err != nil {
				return errors.Wrap(err, "failed to delete image registry "+obj.Metadata.Name)
			}

			return nil
		}

		klog.Info("Deleting image registry " + obj.Metadata.Name)

		err = c.updateStatus(obj, v1.ImageRegistryPhaseDELETED, nil)
		if err != nil {
			return errors.Wrap(err, "failed to update image registry "+obj.Metadata.Name)
		}

		return nil
	}

	defer func() {
		phase := v1.ImageRegistryPhaseCONNECTED
		if err != nil {
			phase = v1.ImageRegistryPhaseFAILED
		}

		updateStatusErr := c.updateStatus(obj, phase, err)
		if updateStatusErr != nil {
			klog.Error(updateStatusErr, "failed to update image registry status")
		}
	}()

	klog.Info("Connect to image registry " + obj.Metadata.Name)

	err = c.connectImageRegistry(obj)
	if err != nil {
		return errors.Wrap(err, "failed to connect image registry "+obj.Metadata.Name)
	}

	return nil
}

func (c *ImageRegistryController) connectImageRegistry(imageRegistry *v1.ImageRegistry) error {
	authConfig := registry.AuthConfig{
		Username:      imageRegistry.Spec.AuthConfig.Username,
		Password:      imageRegistry.Spec.AuthConfig.Password,
		ServerAddress: imageRegistry.Spec.URL,
		IdentityToken: imageRegistry.Spec.AuthConfig.IdentityToken,
		RegistryToken: imageRegistry.Spec.AuthConfig.IdentityToken,
	}

	_, err := c.dockerClient.RegistryLogin(context.Background(), authConfig)
	if err != nil {
		return errors.Wrap(err, "image registry login failed")
	}

	return nil
}

func (c *ImageRegistryController) updateStatus(obj *v1.ImageRegistry, phase v1.ImageRegistryPhase, err error) error {
	obj.Status = v1.ImageRegistryStatus{
		LastTransitionTime: time.Now().Format(time.RFC3339Nano),
		Phase:              phase,
	}
	if err != nil {
		obj.Status.ErrorMessage = err.Error()
	}

	updateStatusErr := c.storage.UpdateImageRegistry(strconv.Itoa(obj.ID), obj)
	if err != nil {
		return errors.Wrap(updateStatusErr, "failed to update image registry "+obj.Metadata.Name)
	}

	return updateStatusErr
}
