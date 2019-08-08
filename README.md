# event-streams-topic

The event-streams-topic operator supports the life cycle management of Topics for EventStreams on IBM Cloud.

## Installation

To install:
```
git clone git@github.ibm.com:seed/event-streams-topic.git
./event-streams-topic/hack/install-topic.sh
```

To uninstall:
```
./event-streams-topic/hack/uninstall-topic.sh
```

## Example

Provided a Kubernetes secret, `binding-messagehub`, which contains credentials for the managed EventStreams service (apiKey, and kafkaAdminURL), the following yaml deploys a Topic named `MyGreatTopic` with the configuration settings that are shown.

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
  
The secret containing the credentials may be created in any way. However, and an easy way to obtain it is by using the IBM Cloud Operator (https://github.com/IBM/cloud-operators):
  
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
When created in this way, the Topic's life cycle is bound to that of the managed service. If the Kubernetes object for the service is deleted, then so is the Topic.
