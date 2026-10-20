// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ironcore-dev/network-operator/api/core/v1alpha1"
)

var _ = Describe("AAA Controller", func() {
	Context("When reconciling a resource", func() {
		var (
			name string
			key  client.ObjectKey
		)

		BeforeEach(func() {
			By("Creating the custom resource for the Kind Device")
			device := &v1alpha1.Device{
				GenerateName: "test-aaa-",
				Namespace:    metav1.NamespaceDefault,
				Spec: v1alpha1.DeviceSpec{
					Endpoint: v1alpha1.Endpoint{
						Address: "192.168.10.2:9339",
					},
					Provider: "test-provider",
				},
			}
			Expect(k8sClient.Create(ctx, device)).To(Succeed())
			name = device.Name
			key = client.ObjectKey{Name: name, Namespace: metav1.NamespaceDefault}

			By("Creating the Secret for TACACS server key")
			secret := &corev1.Secret{
				Name:      name,
				Namespace: metav1.NamespaceDefault,
				StringData: map[string]string{
					"server-key": "supersecretkey",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating the custom resource for the Kind AAA")
			resource := &v1alpha1.AAA{
				Name:      name,
				Namespace: metav1.NamespaceDefault,
				Spec: v1alpha1.AAASpec{
					DeviceRef: v1alpha1.LocalObjectReference{Name: name},
					ServerGroups: []v1alpha1.AAAServerGroup{
						{
							Name: "GR_TACACS",
							Type: v1alpha1.AAAServerGroupTypeTACACS,
							Servers: []v1alpha1.AAAServer{
								{
									Address: "10.0.0.1",
									TACACS: &v1alpha1.AAAServerTACACS{
										Port: 49,
										KeySecretRef: v1alpha1.SecretKeySelector{
											SecretReference: v1alpha1.SecretReference{Name: name},
											Key:             "server-key",
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the AAA resource")
			aaa := &v1alpha1.AAA{}
			aaa.Name = name
			aaa.Namespace = metav1.NamespaceDefault
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, aaa))).To(Succeed())

			By("Verifying the resource is removed from the provider")
			Eventually(func(g Gomega) {
				g.Expect(testDevices.StateFor(name).AAA).To(BeNil(), "Provider AAA should be nil")
			}).Should(Succeed())

			By("Waiting for the AAA to be fully deleted")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, key, &v1alpha1.AAA{})
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())

			By("Cleaning up the Secret")
			secret := &corev1.Secret{}
			secret.Name = name
			secret.Namespace = metav1.NamespaceDefault
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, secret))).To(Succeed())

			By("Cleaning up the Device resource")
			device := &v1alpha1.Device{}
			device.Name = name
			device.Namespace = metav1.NamespaceDefault
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, device))).To(Succeed())
		})

		It("Should successfully reconcile the resource", func() {
			By("Adding a finalizer to the resource")
			Eventually(func(g Gomega) {
				resource := &v1alpha1.AAA{}
				g.Expect(k8sClient.Get(ctx, key, resource)).To(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(resource, v1alpha1.FinalizerName)).To(BeTrue())
			}).Should(Succeed())

			By("Adding the device label to the resource")
			Eventually(func(g Gomega) {
				resource := &v1alpha1.AAA{}
				g.Expect(k8sClient.Get(ctx, key, resource)).To(Succeed())
				g.Expect(resource.Labels).To(HaveKeyWithValue(v1alpha1.DeviceLabel, name))
			}).Should(Succeed())

			By("Adding the device as a owner reference")
			Eventually(func(g Gomega) {
				resource := &v1alpha1.AAA{}
				g.Expect(k8sClient.Get(ctx, key, resource)).To(Succeed())
				g.Expect(resource.OwnerReferences).To(HaveLen(1))
				g.Expect(resource.OwnerReferences[0].Kind).To(Equal("Device"))
				g.Expect(resource.OwnerReferences[0].Name).To(Equal(name))
			}).Should(Succeed())

			By("Updating the resource status")
			Eventually(func(g Gomega) {
				resource := &v1alpha1.AAA{}
				g.Expect(k8sClient.Get(ctx, key, resource)).To(Succeed())
				g.Expect(resource.Status.Conditions).To(HaveLen(3))
				g.Expect(resource.Status.Conditions[0].Type).To(Equal(v1alpha1.ReadyCondition))
				g.Expect(resource.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
				g.Expect(resource.Status.Conditions[1].Type).To(Equal(v1alpha1.ConfiguredCondition))
				g.Expect(resource.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
				g.Expect(resource.Status.Conditions[2].Type).To(Equal(v1alpha1.PausedCondition))
				g.Expect(resource.Status.Conditions[2].Status).To(Equal(metav1.ConditionFalse))
			}).Should(Succeed())

			By("Ensuring the resource is created in the provider")
			Eventually(func(g Gomega) {
				g.Expect(testDevices.StateFor(name).AAA).ToNot(BeNil(), "Provider AAA should not be nil")
				if testDevices.StateFor(name).AAA != nil {
					g.Expect(testDevices.StateFor(name).AAA.Spec.ServerGroups).To(HaveLen(1))
					g.Expect(testDevices.StateFor(name).AAA.Spec.ServerGroups[0].Name).To(Equal("GR_TACACS"))
				}
			}).Should(Succeed())
		})
	})
})
