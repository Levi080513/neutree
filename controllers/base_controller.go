package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Reconciler interface {
	Sync(obj interface{}) error
	ListObjects() ([]interface{}, error)
	ProcessObject(obj interface{}) (interface{}, error)
}

type Controller interface {
	Start(ctx context.Context)
}

type BaseController struct {
	queue        workqueue.RateLimitingInterface
	workers      int
	syncInterval time.Duration
}

func (bc *BaseController) Start(ctx context.Context, r Reconciler) {
	klog.Info("Starting controller")
	defer bc.queue.ShutDown()

	for i := 0; i < bc.workers; i++ {
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			for bc.processNextWorkItem(r) {
			}
		}, time.Second)
	}

	wait.Until(func() {
		if err := bc.reconcileAll(r); err != nil {
			klog.Error(err)
		}
	}, bc.syncInterval, ctx.Done())

	<-ctx.Done()
}

func (bc *BaseController) processNextWorkItem(r Reconciler) bool {
	key, quit := bc.queue.Get()
	if quit {
		return false
	}
	defer bc.queue.Done(key)

	obj, err := r.ProcessObject(key)
	if err != nil {
		klog.Error(err)
		return true
	}

	if err := r.Sync(obj); err != nil {
		klog.Error(err)
	}
	return true
}

func (bc *BaseController) reconcileAll(r Reconciler) error {
	objs, err := r.ListObjects()
	if err != nil {
		return err
	}

	for _, obj := range objs {
		bc.queue.Add(obj)
	}
	return nil
}
