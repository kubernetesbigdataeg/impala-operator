/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bigdatav1alpha1 "github.com/kubernetesbigdataeg/impala-operator/api/v1alpha1"
)

const impalaFinalizer = "bigdata.kubernetesbigdataeg.org/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableImpala represents the status of the Deployment reconciliation
	typeAvailableImpala = "Available"
	// typeDegradedImpala represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedImpala = "Degraded"
)

// ImpalaReconciler reconciles a Impala object
type ImpalaReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=impalas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=impalas/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=impalas/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps;services,verbs=get;list;create;watch
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ImpalaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	//
	// 1. Control-loop: checking if impala CR exists
	//
	// Fetch the Impala instance
	// The purpose is check if the Custom Resource for the Kind Impala
	// is applied on the cluster if not we return nil to stop the reconciliation
	impala := &bigdatav1alpha1.Impala{}
	err := r.Get(ctx, req.NamespacedName, impala)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("impala resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get impala")
		return ctrl.Result{}, err
	}

	//
	// 2. Control-loop: Status to Unknown
	//
	// Let's just set the status as Unknown when no status are available
	if impala.Status.Conditions == nil || len(impala.Status.Conditions) == 0 {
		meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeAvailableImpala, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, impala); err != nil {
			log.Error(err, "Failed to update Impala status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the impala Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, impala); err != nil {
			log.Error(err, "Failed to re-fetch impala")
			return ctrl.Result{}, err
		}
	}

	//
	// 3. Control-loop: Let's add a finalizer
	//
	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(impala, impalaFinalizer) {
		log.Info("Adding Finalizer for Impala")
		if ok := controllerutil.AddFinalizer(impala, impalaFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, impala); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	//
	// 4. Control-loop: Instance marked for deletion
	//
	// Check if the Impala instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isImpalaMarkedToBeDeleted := impala.GetDeletionTimestamp() != nil
	if isImpalaMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(impala, impalaFinalizer) {
			log.Info("Performing Finalizer Operations for Impala before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeDegradedImpala,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", impala.Name)})

			if err := r.Status().Update(ctx, impala); err != nil {
				log.Error(err, "Failed to update Impala status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForImpala(impala)

			// TODO(user): If you add operations to the doFinalizerOperationsForImpala method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the impala Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, impala); err != nil {
				log.Error(err, "Failed to re-fetch impala")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeDegradedImpala,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", impala.Name)})

			if err := r.Status().Update(ctx, impala); err != nil {
				log.Error(err, "Failed to update Impala status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Impala after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(impala, impalaFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Impala")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, impala); err != nil {
				log.Error(err, "Failed to remove finalizer for Impala")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	//
	// 5. Control-loop: Let's deploy/ensure our managed resources for impala
	// - ConfigMap,
	// - Service ClusterIP,
	// - Service ClusterIP NodePort,
	// - Service ClusterIP Worker,
	// - StatefulSet Master
	// - StatefulSet Worker
	//

	// Crea o actualiza ConfigMap
	configMapFound := &corev1.ConfigMap{}
	if err := r.ensureResource(ctx, impala, r.defaultConfigMapForImpala, configMapFound, "impala-config", "ConfigMap"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcMasterFound := &corev1.Service{}
	if err := r.ensureResource(ctx, impala, r.serviceMasterForImpala, svcMasterFound, "impala-master-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcMasterUiFound := &corev1.Service{}
	if err := r.ensureResource(ctx, impala, r.serviceUiForImpala, svcMasterUiFound, "impala-ui-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	svcWorkerFound := &corev1.Service{}
	if err := r.ensureResource(ctx, impala, r.serviceWorkerForImpala, svcWorkerFound, "impala-worker-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// StatefulSet
	stsMasterFound := &appsv1.StatefulSet{}
	if err := r.ensureResource(ctx, impala, r.statefulSetMasterForImpala, stsMasterFound, "impala-master", "StatefulSet"); err != nil {
		return ctrl.Result{}, err
	}

	// StatefulSet
	stsWorkerFound := &appsv1.StatefulSet{}
	if err := r.ensureResource(ctx, impala, r.statefulSetWorkerForImpala, stsWorkerFound, "impala-worker", "StatefulSet"); err != nil {
		return ctrl.Result{}, err
	}

	//
	// 6. Control-loop: Check the number of replicas
	//
	// The CRD API is defining that the Impala type, have a ImpalaSpec.Size field
	// to set the quantity of StatefulSet instances is the desired state on the cluster.
	// Therefore, the following code will ensure the StatefulSet size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	masterSize := impala.Spec.MasterSize
	if stsMasterFound.Spec.Replicas == nil {
		log.Error(nil, "Spec is not initialized for Master StatefulSet", "StatefulSet.Namespace", stsMasterFound.Namespace, "StatefulSet.Name", stsMasterFound.Name)
		return ctrl.Result{}, fmt.Errorf("spec is not initialized for StatefulSet %s/%s", stsMasterFound.Namespace, stsMasterFound.Name)
	}
	if *stsMasterFound.Spec.Replicas != masterSize {
		stsMasterFound.Spec.Replicas = &masterSize
		if err = r.Update(ctx, stsMasterFound); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", stsMasterFound.Namespace, "StatefulSet.Name", stsMasterFound.Name)

			// Re-fetch the impala Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, impala); err != nil {
				log.Error(err, "Failed to re-fetch impala")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeAvailableImpala,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", impala.Name, err)})

			if err := r.Status().Update(ctx, impala); err != nil {
				log.Error(err, "Failed to update Impala status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	workerSize := impala.Spec.WorkerSize
	if stsWorkerFound.Spec.Replicas == nil {
		log.Error(nil, "Spec is not initialized for Worker StatefulSet", "StatefulSet.Namespace", stsWorkerFound.Namespace, "StatefulSet.Name", stsWorkerFound.Name)
		return ctrl.Result{}, fmt.Errorf("spec is not initialized for StatefulSet %s/%s", stsWorkerFound.Namespace, stsWorkerFound.Name)
	}
	if *stsWorkerFound.Spec.Replicas != workerSize {
		stsWorkerFound.Spec.Replicas = &workerSize
		if err = r.Update(ctx, stsWorkerFound); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", stsWorkerFound.Namespace, "StatefulSet.Name", stsWorkerFound.Name)

			if err := r.Get(ctx, req.NamespacedName, impala); err != nil {
				log.Error(err, "Failed to re-fetch impala")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeAvailableImpala,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", impala.Name, err)})

			if err := r.Status().Update(ctx, impala); err != nil {
				log.Error(err, "Failed to update Impala status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	//
	// 7. Control-loop: Let's update the status
	//
	// The following implementation will update the status
	meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeAvailableImpala,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", impala.Name, masterSize)})

	if err := r.Status().Update(ctx, impala); err != nil {
		log.Error(err, "Failed to update Impala status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeImpala will perform the required operations before delete the CR.
func (r *ImpalaReconciler) doFinalizerOperationsForImpala(cr *bigdatav1alpha1.Impala) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *ImpalaReconciler) defaultConfigMapForImpala(Impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	configMapData := make(map[string]string, 0)
	impalaEnv := `
	export IMPALA__impaladaemon__abort_on_config_error=false
    export IMPALA__impaladaemon__log_dir=/var/log/impala
    export IMPALA__impaladaemon__state_store_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
    export IMPALA__impaladaemon__catalog_service_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
    export IMPALA__impaladaemon__admission_service_host=impala-master-0.impala-master-svc.kudu.svc.cluster.local
    export IMPALA__impaladaemon__kudu_master_hosts=kudu-master-0.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.kudu.svc.cluster.local:7051
    export IMPALA__impaladaemon__enable_legacy_avx_support=true
    export IMPALA__impaladaemon__statestore_subscriber_use_resolved_address=true
    export IMPALA__impalacatalog__kudu_master_hosts=kudu-master-0.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.kudu.svc.cluster.local:7051
    export IMPALA__impalacatalog__log_dir=/var/log/impala
    export IMPALA__impalacatalog__enable_legacy_avx_support=true
    export IMPALA__impalastatestore__kudu_master_hosts=kudu-master-0.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.kudu.svc.cluster.local:7051
    export IMPALA__impalastatestore__log_dir=/var/log/impala
    export IMPALA__impalastatestore__enable_legacy_avx_support=true
    export IMPALA__impalaadmission__kudu_master_hosts=kudu-master-0.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-1.kudu-master-svc.kudu.svc.cluster.local:7051,kudu-master-2.kudu-master-svc.kudu.svc.cluster.local:7051
    export IMPALA__impalaadmission__log_dir=/var/log/impala
    export IMPALA__impalaadmission__enable_legacy_avx_support=true
    export IMPALA__hivesite__javax_jdo_option_ConnectionURL="jdbc:postgresql://postgresql-svc.kudu.svc.cluster.local:5432/metastore"
    export IMPALA__hivesite__javax_jdo_option_ConnectionDriverName="org.postgresql.Driver"
    export IMPALA__hivesite__javax_jdo_option_ConnectionUserName="postgres"
    export IMPALA__hivesite__javax_jdo_option_ConnectionPassword="postgres"
    export IMPALA__hivesite__metastore_expression_proxy="org.apache.hadoop.hive.metastore.DefaultPartitionExpressionProxy"
    export IMPALA__hivesite__metastore_task_threads_always="org.apache.hadoop.hive.metastore.events.EventCleanerTask,org.apache.hadoop.hive.metastore.MaterializationsCacheCleanerTask"
    export IMPALA__hivesite__datanucleus_autoCreateSchema="false"
    export IMPALA__hivesite__hive_metastore_uris="thrift://hive-svc.kudu.svc.cluster.local:9083"
    export IMPALA__hivesite__hive_metastore_warehouse_dir="/var/lib/hive/warehouse"
    export IMPALA__hivesite__hive_metastore_transactional_event_listeners="org.apache.hive.hcatalog.listener.DbNotificationListener,org.apache.kudu.hive.metastore.KuduMetastorePlugin"
    export IMPALA__hivesite__hive_metastore_disallow_incompatible_col_type_changes="false"
    export IMPALA__hivesite__hive_metastore_dml_events="true"
    export IMPALA__hivesite__hive_metastore_event_db_notification_api_auth="false"
    export IMPALA__coresite__fs_defaultFS="hdfs://hdfs-k8s"
    export IMPALA__coresite__ha_zookeeper_quorum="zk-0.zk-hs.default.svc.cluster.local:2181,zk-1.zk-hs.default.svc.cluster.local:2181,zk-2.zk-hs.default.svc.cluster.local:2181"
    export IMPALA__hdfssite__dfs_nameservices="hdfs-k8s"
    export IMPALA__hdfssite__dfs_ha_namenodes_hdfs___k8s="nn0,nn1"
    export IMPALA__hdfssite__dfs_namenode_rpc___address_hdfs___k8s_nn0="hdfs-namenode-0.hdfs-namenode-svc.default.svc.cluster.local:8020"
    export IMPALA__hdfssite__dfs_namenode_rpc___address_hdfs___k8s_nn1="hdfs-namenode-1.hdfs-namenode-svc.default.svc.cluster.local:8020"
    export IMPALA__hdfssite__dfs_namenode_http___address_hdfs___k8s_nn0="hdfs-namenode-0.hdfs-namenode-svc.default.svc.cluster.local:50070"
    export IMPALA__hdfssite__dfs_namenode_http___address_hdfs___k8s_nn1="hdfs-namenode-1.hdfs-namenode-svc.default.svc.cluster.local:50070"
    export IMPALA__hdfssite__dfs_namenode_shared_edits_dir="qjournal://hdfs-journalnode-0.hdfs-journalnode-svc.default.svc.cluster.local:8485;hdfs-journalnode-1.hdfs-journalnode-svc.default.svc.cluster.local:8485;hdfs-journalnode-2.hdfs-journalnode-svc.default.svc.cluster.local:8485/hdfs-k8s"
    export IMPALA__hdfssite__dfs_ha_automatic___failover_enabled="true"
    export IMPALA__hdfssite__dfs_ha_fencing_methods="shell(/bin/true)"
    export IMPALA__hdfssite__dfs_journalnode_edits_dir="/hadoop/dfs/journal"
    export IMPALA__hdfssite__dfs_client_failover_proxy_provider_hdfs___k8s="org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider"
    export IMPALA__hdfssite__dfs_namenode_name_dir="file:///hadoop/dfs/name"
    export IMPALA__hdfssite__dfs_namenode_datanode_registration_ip___hostname___check="false"
    export IMPALA__hdfssite__dfs_datanode_data_dir="/hadoop/dfs/data"
    export IMPALA__hdfssite__dfs_client_read_shortcircuit="true"
    export IMPALA__hdfssite__dfs_domain_socket_path="/var/run/hdfs-sockets/dn"
    export IMPALA__hdfssite__dfs_client_file___block___storage___locations_timeout_millis="10000"
    export IMPALA__hdfssite__dfs_datanode_hdfs___blocks___metadata_enabled="true"
	`

	configMapData["impala.env"] = impalaEnv
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Impala.Namespace,
		},
		Data: configMapData,
	}

	if err := ctrl.SetControllerReference(Impala, configMap, r.Scheme); err != nil {
		return nil, err
	}

	return configMap, nil
}

func (r *ImpalaReconciler) serviceMasterForImpala(Impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	labels := labelsForImpala(Impala.Name, "impala-master")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Impala.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "be-port",
					Port: 22000,
				},
				{
					Name: "impalad-store",
					Port: 23000,
				},
				{
					Name: "catalogd-store",
					Port: 23020,
				},
				{
					Name: "state-store",
					Port: 24000,
				},
				{
					Name: "catalog-service",
					Port: 26000,
				},
				{
					Name: "beewax-service",
					Port: 21000,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(Impala, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

func (r *ImpalaReconciler) serviceUiForImpala(Impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	labels := labelsForImpala(Impala.Name, "impala-master")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Impala.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:     "impalad-ui",
				Port:     25000,
				NodePort: 30215,
			}},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	if err := ctrl.SetControllerReference(Impala, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

func (r *ImpalaReconciler) serviceWorkerForImpala(Impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	labels := labelsForImpala(Impala.Name, "impala-worker")
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Impala.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "be-port",
					Port: 22000,
				},
				{
					Name: "impalad-store",
					Port: 23000,
				},
				{
					Name: "beewax-service",
					Port: 21000,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(Impala, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

// statefulSetForImpala returns a Impala StatefulSet object
func (r *ImpalaReconciler) statefulSetMasterForImpala(impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	labels := labelsForImpala(impala.Name, "impala-master")

	replicas := impala.Spec.MasterSize

	// Get the Operand image
	image, err := imageForImpala()
	if err != nil {
		return nil, err
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: impala.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "impala-master-svc",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          "RollingUpdate",
				RollingUpdate: nil,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:           image,
							Name:            "impala",
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"master"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "be-port",
									ContainerPort: 22000,
								},
								{
									Name:          "impalad-store",
									ContainerPort: 23000,
								},
								{
									Name:          "catalogd-store",
									ContainerPort: 23020,
								},
								{
									Name:          "state-store",
									ContainerPort: 24000,
								},
								{
									Name:          "catalog-service",
									ContainerPort: 26000,
								},
								{
									Name:          "beewax-service",
									ContainerPort: 21000,
								},
								{
									Name:          "impalad-ui",
									ContainerPort: 25000,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IMPALA_HOME",
									Value: "/opt/impala",
								},
								{
									Name:  "JAVA_HOME",
									Value: "/usr/lib/jvm/java/",
								},
								{
									Name:  "CLASSPATH",
									Value: "/opt/impala/lib/*:/opt/hive/lib/*",
								},
								{
									Name:  "HADOOP_HOME",
									Value: "/opt/hadoop",
								},
								{
									Name:  "HIVE_HOME",
									Value: "/opt/hive",
								},
								{
									Name:  "HIVE_CONF_DIR",
									Value: "/opt/hive/conf",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
								{
									Name:      "impala-config-volume",
									MountPath: "/etc/environments",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "logs-impalad",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/impala/impalad.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "logs-catalogd",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/impala/catalogd.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "logs-admissiond",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/impala/admissiond.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "logs-statestored",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/impala/statestored.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "impala-config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "impala-config",
									},
								},
							},
						},
						{
							Name: "impala-logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(impala, sts, r.Scheme); err != nil {
		return nil, err
	}
	return sts, nil
}

func (r *ImpalaReconciler) statefulSetWorkerForImpala(impala *bigdatav1alpha1.Impala, resourceName string) (client.Object, error) {

	labels := labelsForImpala(impala.Name, "impala-worker")

	replicas := impala.Spec.WorkerSize

	// Get the Operand image
	image, err := imageForImpala()
	if err != nil {
		return nil, err
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: impala.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "impala-worker-svc",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          "RollingUpdate",
				RollingUpdate: nil,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:           image,
							Name:            "impala",
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"worker"},
							Ports: []corev1.ContainerPort{
								{
									Name:          "be-port",
									ContainerPort: 22000,
								},
								{
									Name:          "impalad-store",
									ContainerPort: 23000,
								},
								{
									Name:          "beewax-service",
									ContainerPort: 21000,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "IMPALA_HOME",
									Value: "/opt/impala",
								},
								{
									Name:  "JAVA_HOME",
									Value: "/usr/lib/jvm/java/",
								},
								{
									Name:  "CLASSPATH",
									Value: "/opt/impala/lib/*:/opt/hive/lib/*",
								},
								{
									Name:  "HADOOP_HOME",
									Value: "/opt/hadoop",
								},
								{
									Name:  "HIVE_HOME",
									Value: "/opt/hive",
								},
								{
									Name:  "HIVE_CONF_DIR",
									Value: "/opt/hive/conf",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
								{
									Name:      "impala-config-volume",
									MountPath: "/etc/environments",
								},
							},
						},
						{
							Image: "busybox:1.28",
							Name:  "logs-impalad",
							Args:  []string{"/bin/sh", "-c", "tail -n+1 -F /var/log/impala/impalad.INFO"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "impala-logs",
									MountPath: "/var/log/impala/",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "impala-config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "impala-config",
									},
								},
							},
						},
						{
							Name: "impala-logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(impala, sts, r.Scheme); err != nil {
		return nil, err
	}
	return sts, nil
}

// labelsForImpala returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForImpala(name string, app string) map[string]string {
	var imageTag string
	image, err := imageForImpala()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{
		"app.kubernetes.io/name":       "Impala",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "impala-operator",
		"app.kubernetes.io/created-by": "controller-manager",
		"app":                          app,
	}
}

// imageForImpala gets the Operand image which is managed by this controller
// from the IMPALA_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForImpala() (string, error) {
	var imageEnvVar = "IMPALA_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *ImpalaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bigdatav1alpha1.Impala{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *ImpalaReconciler) ensureResource(ctx context.Context, impala *bigdatav1alpha1.Impala, createResourceFunc func(*bigdatav1alpha1.Impala, string) (client.Object, error), foundResource client.Object, resourceName string, resourceType string) error {
	log := log.FromContext(ctx)
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: impala.Namespace}, foundResource)
	if err != nil && apierrors.IsNotFound(err) {
		resource, err := createResourceFunc(impala, resourceName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to define new %s resource for Impala", resourceType))

			// The following implementation will update the status
			meta.SetStatusCondition(&impala.Status.Conditions, metav1.Condition{Type: typeAvailableImpala,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create %s for the custom resource (%s): (%s)", resourceType, impala.Name, err)})

			if err := r.Status().Update(ctx, impala); err != nil {
				log.Error(err, "Failed to update Impala status")
				return err
			}

			return err
		}

		log.Info(fmt.Sprintf("Creating a new %s", resourceType),
			fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())

		if err = r.Create(ctx, resource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to create new %s", resourceType),
				fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())
			return err
		}

		time.Sleep(5 * time.Second)

		if err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: impala.Namespace}, foundResource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to get newly created %s", resourceType))
			return err
		}

	} else if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get %s", resourceType))
		return err
	}

	return nil
}
