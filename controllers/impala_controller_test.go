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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bigdatav1alpha1 "github.com/kubernetesbigdataeg/impala-operator/api/v1alpha1"
)

var _ = Describe("Impala controller", func() {
	Context("Impala controller test", func() {

		const ImpalaName = "test-impala"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ImpalaName,
				Namespace: ImpalaName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: ImpalaName, Namespace: ImpalaName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("IMPALA_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("IMPALA_IMAGE")
		})

		It("should successfully reconcile a custom resource for Impala", func() {
			By("Creating the custom resource for the Kind Impala")
			impala := &bigdatav1alpha1.Impala{}
			err := k8sClient.Get(ctx, typeNamespaceName, impala)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				impala := &bigdatav1alpha1.Impala{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ImpalaName,
						Namespace: namespace.Name,
					},
					Spec: bigdatav1alpha1.ImpalaSpec{
						MasterSize: 1,
						WorkerSize: 3,
					},
				}

				err = k8sClient.Create(ctx, impala)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &bigdatav1alpha1.Impala{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			impalaReconciler := &ImpalaReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = impalaReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if StatefulSet was successfully created in the reconciliation")
			statefulSets := []string{"impala-master", "impala-worker"}
			for _, statefulSetName := range statefulSets {
				By(fmt.Sprintf("Checking if %s StatefulSet was successfully created in the reconciliation", statefulSetName))
				statefulSetNamespaceName := types.NamespacedName{Name: statefulSetName, Namespace: ImpalaName}
				Eventually(func() error {
					found := &appsv1.StatefulSet{}
					return k8sClient.Get(ctx, statefulSetNamespaceName, found)
				}, time.Minute, time.Second).Should(Succeed())
			}

			By("Checking the latest Status Condition added to the Impala instance")
			Eventually(func() error {
				if impala.Status.Conditions != nil && len(impala.Status.Conditions) != 0 {
					latestStatusCondition := impala.Status.Conditions[len(impala.Status.Conditions)-1]
					expectedLatestStatusCondition := metav1.Condition{Type: typeAvailableImpala,
						Status: metav1.ConditionTrue, Reason: "Reconciling",
						Message: fmt.Sprintf("StatefulSet for custom resource (%s) with %d replicas created successfully", impala.Name, impala.Spec.MasterSize)}
					if latestStatusCondition != expectedLatestStatusCondition {
						return fmt.Errorf("The latest status condition added to the impala instance is not as expected")
					}
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())
		})
	})
})
