/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package topic

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	ibmcloud "github.com/ibm/cloud-operators/pkg/apis/ibmcloud/v1alpha1"
	rcontext "github.com/ibm/cloud-operators/pkg/context"
	ibmcloudv1alpha1 "github.com/ibm/event-streams-topic/pkg/apis/ibmcloud/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var logt = logf.Log.WithName("topic")

const topicFinalizer = "topic.ibmcloud.ibm.com"

// ContainsFinalizer checks if the instance contains service finalizer
func ContainsFinalizer(instance *ibmcloudv1alpha1.Topic) bool {
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if strings.Contains(finalizer, topicFinalizer) {
			return true
		}
	}
	return false
}

// DeleteFinalizer delete service finalizer
func DeleteFinalizer(instance *ibmcloudv1alpha1.Topic) []string {
	var result []string
	for _, finalizer := range instance.ObjectMeta.Finalizers {
		if finalizer == topicFinalizer {
			continue
		}
		result = append(result, finalizer)
	}
	return result
}

// Add creates a new Topic Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this ibmcloud.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTopic{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("topic-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 33})
	if err != nil {
		return err
	}

	// Watch for changes to Topic
	err = c.Watch(&source.Kind{Type: &ibmcloudv1alpha1.Topic{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by Binding - change this for objects you create
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &ibmcloud.Binding{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileTopic{}

// ReconcileTopic reconciles a Topic object
type ReconcileTopic struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Topic object and makes changes based on the state read
// and what is in the Topic.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=topics/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ibmcloud.ibm.com,resources=bindings,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileTopic) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := rcontext.New(r.Client, request)

	// Fetch the Topic instance
	instance := &ibmcloudv1alpha1.Topic{}
	err := r.Get(context.Background(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Set the Status field for the first time
	if reflect.DeepEqual(instance.Status, ibmcloudv1alpha1.TopicStatus{}) {
		instance.Status.State = "Pending"
		instance.Status.Message = "Processing Resource"
		if err := r.Status().Update(context.Background(), instance); err != nil {
			return reconcile.Result{}, nil
		}
	}

	// Check that the spec is well-formed
	if !isWellFormed(*instance) {
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return reconcile.Result{}, nil
		}
		return r.updateStatusError(instance, "Failed", fmt.Errorf("The spec is not well-formed").Error())
	}

	kafkaAdminURL, apiKey, ret, err := getKafkaAdminInfo(r, ctx, instance)
	if err != nil {
		if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
			// In this case it is enough to simply remove the finalizer:
			instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error removing finalizers", "in deletion", err.Error())
				// No further action required, object was modified, another reconcile will finish the job.
			}
			return reconcile.Result{}, nil
		}
		logt.Info("Kafka admin URL and/or APIKey not found", instance.Name, err.Error())
		return ret, err
	}

	// Delete if necessary
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// Instance is not being deleted, add the finalizer if not present
		if !ContainsFinalizer(instance) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, topicFinalizer)
			if err := r.Update(context.Background(), instance); err != nil {
				logt.Info("Error adding finalizer", instance.Name, err.Error())
				return reconcile.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if ContainsFinalizer(instance) {
			result, err := deleteTopic(kafkaAdminURL, apiKey, instance)
			if err != nil {
				logt.Info("Error deleting topic", "in deletion", err.Error())
				return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
			}
			if result.StatusCode == 202 || result.StatusCode == 404 { // deletion succeeded or topic does not exist
				// remove our finalizer from the list and update it.
				err := r.deleteConfigMap(instance)
				if err != nil {
					return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
				}
				instance.ObjectMeta.Finalizers = DeleteFinalizer(instance)
				if err := r.Update(context.Background(), instance); err != nil {
					logt.Info("Error removing finalizers", "in deletion", err.Error())
				}
				return reconcile.Result{}, nil
			}
		}
	}

	logt.Info("Getting", "topic", instance.Name)
	result, err := getTopic(kafkaAdminURL, apiKey, instance)
	logt.Info("Result of Get", instance.Name, result.StatusCode)
	if result.StatusCode == 405 && strings.Contains(result.Body, "Method Not Allowed") {
		logt.Info("Trying to create", "topic", instance.Name)
		// This must be a CF Messagehub, does not support GET, test by trying to create
		result, err = createTopic(ctx, kafkaAdminURL, apiKey, instance)
		logt.Info("Creation result", instance.Name, result)
		if result.StatusCode == 200 { // Success
			logt.Info("Topic created", "success", instance.Name)
			return r.updateStatusOnline(kafkaAdminURL, apiKey, instance)
		} else if strings.Contains(result.Body, "already exists") {
			logt.Info("Topic already exists", "success", instance.Name)
			return r.updateStatusOnline(kafkaAdminURL, apiKey, instance)
		}
		if err != nil {
			logt.Info("Topic creation", "error", err.Error())
			return r.updateStatusError(instance, "Failed", err.Error())
		}
		logt.Info("Topic creation error", instance.Name, result.Body)
		return r.updateStatusError(instance, "Failed", result.Body)

	} else if result.StatusCode == 200 {
		// TODO: check that the configuration is the same
		return r.updateStatusOnline(kafkaAdminURL, apiKey, instance)

	} else if result.StatusCode == 404 && strings.Contains(result.Body, "unable to get topic details") {
		// Need to create the topic
		result, err = createTopic(ctx, kafkaAdminURL, apiKey, instance)
		if err != nil {
			logt.Info("Topic creation error", instance.Name, err.Error())
			return r.updateStatusError(instance, "Failed", err.Error())
		}
		if result.StatusCode != 200 && result.StatusCode != 202 {
			logt.Info("Topic creation error", instance.Name, result.StatusCode)
			return r.updateStatusError(instance, "Failed", result.Body)
		}
		return r.updateStatusOnline(kafkaAdminURL, apiKey, instance)
	} else {
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 20}, nil
	}
}

func (r *ReconcileTopic) updateStatusError(instance *ibmcloudv1alpha1.Topic, state string, message string) (reconcile.Result, error) {
	logt.Info(message)
	if strings.Contains(message, "dial tcp: lookup iam.cloud.ibm.com: no such host") || strings.Contains(message, "dial tcp: lookup login.ng.bluemix.net: no such host") {
		// This means that the IBM Cloud server is under too much pressure, we need to back up
		if instance.Status.State != state {
			instance.Status.Message = "Temporarily lost connection to server"
			instance.Status.State = "Pending"
			if err := r.Status().Update(context.Background(), instance); err != nil {
				logt.Info("Error updating status", state, err.Error())
			}
		}
		return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 3}, nil
	}
	if instance.Status.State != state {
		instance.Status.State = state
		instance.Status.Message = message
		if err := r.Status().Update(context.Background(), instance); err != nil {
			logt.Info("Error updating status", state, err.Error())
			return reconcile.Result{}, nil
		}

	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 1}, nil
}

func (r *ReconcileTopic) updateStatusOnline(kafkaAdminURL string, apiKey string, instance *ibmcloudv1alpha1.Topic) (reconcile.Result, error) {
	instance.Status.State = "Online"
	instance.Status.Message = "Online"
	err := r.Status().Update(context.Background(), instance)
	if err != nil {
		logt.Info("Failed to update online status, will delete external resource ", instance.ObjectMeta.Name, err.Error())
		res, err := deleteTopic(kafkaAdminURL, apiKey, instance)
		if err != nil || (res.StatusCode != 202 && res.StatusCode != 404) {
			logt.Info("Failed to delete external resource, operator state and external resource might be in an inconsistent state", instance.ObjectMeta.Name, err.Error())
		}
		err = r.deleteConfigMap(instance)
		if err != nil {
			logt.Info("Failed to delete configMap", instance.ObjectMeta.Name, err.Error())
			return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
		}
	} else {
		// Now check or create a configMap with info about the topic
		err = r.checkConfigMap(instance)
		if err != nil {
			return r.updateStatusError(instance, "Failed", err.Error())
		}
	}
	return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
}

// This function checks that a configMap exists with the same name and namespace as the Topic object, with info about external
func (r *ReconcileTopic) checkConfigMap(instance *ibmcloudv1alpha1.Topic) error {
	configMap := &corev1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}, configMap)
	if err != nil {
		// ConfigMap does not exist, recreate it
		err = r.createConfigMap(instance)
		if err != nil {
			return err
		}
	} else {
		// ConfigMap exists, check the content
		if configMap.Data["Topic"] != instance.Spec.TopicName {
			// Content not the same, update configMap
			configMap.Data["Topic"] = instance.Spec.TopicName
			if err := r.Update(context.Background(), configMap); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ReconcileTopic) createConfigMap(instance *ibmcloudv1alpha1.Topic) error {
	logt.Info("Creating ", "ConfigMap", instance.ObjectMeta.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.ObjectMeta.Name,
			Namespace: instance.Namespace,
		},
		Data: map[string]string{
			"Topic": instance.Spec.TopicName,
		},
	}
	if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
		return err
	}
	if err := r.Create(context.Background(), configMap); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileTopic) deleteConfigMap(instance *ibmcloudv1alpha1.Topic) error {
	logt.Info("Deleting ", "configmap", instance.Name)
	configMap := &corev1.ConfigMap{}
	err := r.Get(context.Background(), types.NamespacedName{Name: instance.ObjectMeta.Name, Namespace: instance.ObjectMeta.Namespace}, configMap)
	if err != nil {
		// nothing to do
		return nil
	}

	// delete the configMap
	if err := r.Delete(context.Background(), configMap); err != nil {
		return err
	}

	return nil
}

func isWellFormed(instance ibmcloudv1alpha1.Topic) bool {
	if instance.Spec.BindingFrom.Name != "" {
		if instance.Spec.APIKey != nil {
			return false
		}
		if instance.Spec.KafkaAdminURL != nil {
			return false
		}
	}

	if instance.Spec.APIKey != nil && instance.Spec.KafkaAdminURL == nil {
		return false
	}

	if instance.Spec.APIKey != nil && instance.Spec.KafkaAdminURL == nil {
		return false
	}
	return true
}
