# event-streams-topic

The event-streams-topic operator supports the life cycle management of Topics for EventStreams on IBM Cloud.

## Requirements

The operator can be installed on any Kubernetes cluster with version >= 1.11. Before installing, make sure
you have installed [kubectl CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl/) and it is configured to access your cluster.

This operator can be used with IBM Event Streams instances provisioned by the IBM Cloud UI or CLI, or
by the [IBM Cloud Operator](https://github.com/IBM/cloud-operators).

## Installation

To install the latest release open a terminal and run:
```
curl -sL https://raw.githubusercontent.com/IBM/event-streams-topic/master/hack/install-topic.sh | bash
```

To uninstall:
```
curl -sL https://raw.githubusercontent.com/IBM/event-streams-topic/master/hack/uninstall-topic.sh | bash
```

## Example

Assuming you have created a Kubernetes secret, `binding-messagehub`, which contains credentials for the managed EventStreams service (apiKey, and kafkaAdminURL), the following yaml deploys a Topic named `MyGreatTopic` with the configuration settings that are shown.

```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Topic
metadata:
  name: mytopic
spec:
  apiKey:
    secretKeyRef:
      name: binding-messagehub
      key: api_key
  kafkaAdminUrl:
    secretKeyRef:
      name: binding-messagehub
      key: kafka_admin_url
  topicName: MyGreatTopic
  numPartitions: 3
  replicationFactor : 3
  configs :
    - name: retentionMs
      value: 2592000000
  ```
  
The secret containing the credentials may be created in any way. However, an easy way to obtain it is by using the [IBM Cloud Operator](https://github.com/IBM/cloud-operators):
  
```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Service
metadata:
  name: mymessagehub
spec:
  plan: standard
  serviceClass: messagehub
---
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Binding
metadata:
  name: binding-messagehub
spec:
  serviceName: mymessagehub
```

If using the IBM Cloud Operator, then the Topic yaml may also be written more simply as:
```
apiVersion: ibmcloud.ibm.com/v1alpha1
kind: Topic
metadata:
  name: mytopic
spec:
  bindingFrom:
    name: binding-messagehub
  topicName: exampleone
  numPartitions: 3
  replicationFactor : 3
  configs :
    - name: retentionMs
      value: 2592000000
```

This automatically extracts the apiKey and kafkaAdminUrl from the secret corresponding to the binding object.
When created this way, the Topic's life cycle is bound to the one of the managed service. If the Kubernetes object for the service is deleted, then so is the Topic.
