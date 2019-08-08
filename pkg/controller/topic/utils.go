package topic

import (
	"encoding/json"
	"fmt"
	"time"

	rcontext "github.com/ibm/cloud-operators/pkg/context"
	binding "github.com/ibm/cloud-operators/pkg/controller/binding"
	common "github.com/ibm/cloud-operators/pkg/controller/common"
	keyvalue "github.com/ibm/cloud-operators/pkg/lib/keyvalue/v1"
	ibmcloudv1alpha1 "github.com/ibm/event-streams-topic/pkg/apis/ibmcloud/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Create is a type used for passing parameters to Rest calls for creation
type Create struct {
	Name              string                 `json:"name"`
	Partitions        int32                  `json:"partitions"`
	ReplicationFactor int32                  `json:"replicationFactor,omitempty"`
	Configs           map[string]interface{} `json:"configs,omitempty"`
}

func getTopic(kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics/%s", kafkaAdminURL, topic.Spec.TopicName)
	ret, resterr := common.RestCallFunc(epString, nil, "GET", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func deleteTopic(kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics/%s", kafkaAdminURL, topic.Spec.TopicName)
	ret, resterr := common.RestCallFunc(epString, nil, "DELETE", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func createTopic(ctx rcontext.Context, kafkaAdminURL string, apiKey string, topic *ibmcloudv1alpha1.Topic) (common.RestResult, error) {
	epString := fmt.Sprintf("%s/admin/topics", kafkaAdminURL)
	var topicObj Create
	topicObj.Name = topic.Spec.TopicName
	if topicObj.Partitions == 0 {
		topicObj.Partitions = 1
	} else {
		topicObj.Partitions = topic.Spec.NumPartitions
	}
	topicObj.ReplicationFactor = topic.Spec.ReplicationFactor

	if topic.Spec.Configs != nil {
		configMap := make(map[string]interface{})
		for _, kv := range topic.Spec.Configs {
			kvJSON, err := kv.ToJSON(ctx)
			if err != nil {
				return common.RestResult{}, err
			}
			configMap[kv.Name] = kvJSON
		}
		topicObj.Configs = configMap
	}
	topicBlob, _ := json.Marshal(&topicObj)

	// ("Topic Creation content ", "topicBlob", topicBlob)
	ret, resterr := common.RestCallFunc(epString, topicBlob, "POST", "X-Auth-Token", apiKey, true)
	return ret, resterr
}

func getKafkaAdminInfo(r *ReconcileTopic, ctx rcontext.Context, instance *ibmcloudv1alpha1.Topic) (string, string, reconcile.Result, error) {
	bindingNamespace := instance.Namespace
	if instance.Spec.BindingFrom.Namespace != "" {
		bindingNamespace = instance.Spec.BindingFrom.Namespace
	}

	if instance.Spec.BindingFrom.Name != "" { // BindingFrom is used
		bindingInstance, err := binding.GetBinding(r, instance.Spec.BindingFrom.Name, bindingNamespace)
		if err != nil {
			logt.Info("Binding not found", instance.Spec.BindingFrom.Name, bindingNamespace)
			return "", "", reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}

		if err := controllerutil.SetControllerReference(bindingInstance, instance, r.scheme); err != nil {
			return "", "", reconcile.Result{}, err
		}
		secret, err := binding.GetSecret(r, bindingInstance)
		if err != nil {
			logt.Info("Secret not found", instance.Spec.BindingFrom.Name, bindingNamespace)
			if bindingInstance.Status.State == "Online" {
				return "", "", reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
			return "", "", reconcile.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err

		}
		kafkaAdminURL, ok := secret.Data["kafka_admin_url"]
		if !ok {
			return "", "", reconcile.Result{}, fmt.Errorf("missing kafka_admin_url")
		}

		apiKey, ok := secret.Data["api_key"]
		if !ok {
			return "", "", reconcile.Result{}, fmt.Errorf("missing kafka_admin_url")
		}
		return string(kafkaAdminURL), string(apiKey), reconcile.Result{}, nil

	}

	// BindingFrom is not used
	apiKey, err := keyvalue.ValueToJSON(ctx, *instance.Spec.APIKey)
	if err != nil {
		return "", "", reconcile.Result{}, fmt.Errorf("Missing apiKey")
	}

	kafkaAdminURL, err := keyvalue.ValueToJSON(ctx, *instance.Spec.KafkaAdminURL)
	if err != nil {
		return "", "", reconcile.Result{}, fmt.Errorf("Missing kafkaAdminUrl")
	}
	return kafkaAdminURL.(string), apiKey.(string), reconcile.Result{}, nil

}
