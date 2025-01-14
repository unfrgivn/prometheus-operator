// Copyright 2016 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package framework

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func MakeBasicIngress(serviceName string, servicePort int) *networkv1.Ingress {
	return &networkv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "monitoring",
		},
		Spec: networkv1.IngressSpec{
			Rules: []networkv1.IngressRule{
				{
					IngressRuleValue: networkv1.IngressRuleValue{
						HTTP: &networkv1.HTTPIngressRuleValue{
							Paths: []networkv1.HTTPIngressPath{
								{
									Backend: networkv1.IngressBackend{
										Service: &networkv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkv1.ServiceBackendPort{
												Number: int32(servicePort),
											},
										},
									},
									Path: "/metrics",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (f *Framework) CreateIngress(namespace string, i *networkv1.Ingress) error {
	_, err := f.KubeClient.NetworkingV1().Ingresses(namespace).Create(f.Ctx, i, metav1.CreateOptions{})
	return errors.Wrap(err, fmt.Sprintf("creating ingress %v failed", i.Name))
}

func (f *Framework) SetupNginxIngressControllerIncDefaultBackend(namespace string) error {
	// Create Nginx Ingress Replication Controller
	if err := f.createReplicationControllerViaYml(namespace, "./framework/resources/nxginx-ingress-controller.yml"); err != nil {
		return errors.Wrap(err, "creating nginx ingress replication controller failed")
	}

	// Create Default HTTP Backend Replication Controller
	if err := f.createReplicationControllerViaYml(namespace, "./framework/resources/default-http-backend.yml"); err != nil {
		return errors.Wrap(err, "creating default http backend replication controller failed")
	}

	// Create Default HTTP Backend Service
	manifest, err := os.Open("./framework/resources/default-http-backend-service.yml")
	if err != nil {
		return errors.Wrap(err, "reading default http backend service yaml failed")
	}

	service := v1.Service{}
	err = yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&service)
	if err != nil {
		return errors.Wrap(err, "decoding http backend service yaml failed")
	}

	_, err = f.KubeClient.CoreV1().Services(namespace).Create(f.Ctx, &service, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("creating http backend service %v failed", service.Name))
	}
	if err := f.WaitForServiceReady(namespace, service.Name); err != nil {
		return errors.Wrap(err, fmt.Sprintf("waiting for http backend service %v timed out", service.Name))
	}

	return nil
}

func (f *Framework) DeleteNginxIngressControllerIncDefaultBackend(namespace string) error {
	// Delete Nginx Ingress Replication Controller
	if err := f.deleteReplicationControllerViaYml(namespace, "./framework/resources/nxginx-ingress-controller.yml"); err != nil {
		return errors.Wrap(err, "deleting nginx ingress replication controller failed")
	}

	// Delete Default HTTP Backend Replication Controller
	if err := f.deleteReplicationControllerViaYml(namespace, "./framework/resources/default-http-backend.yml"); err != nil {
		return errors.Wrap(err, "deleting default http backend replication controller failed")
	}

	// Delete Default HTTP Backend Service
	manifest, err := os.Open("./framework/resources/default-http-backend-service.yml")
	if err != nil {
		return errors.Wrap(err, "reading default http backend service yaml failed")
	}

	service := v1.Service{}
	err = yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&service)
	if err != nil {
		return errors.Wrap(err, "decoding http backend service yaml failed")
	}

	if err := f.KubeClient.CoreV1().Services(namespace).Delete(f.Ctx, service.Name, metav1.DeleteOptions{}); err != nil {
		return errors.Wrap(err, fmt.Sprintf("deleting http backend service %v failed", service.Name))
	}

	return nil
}

func (f *Framework) GetIngressIP(namespace string, ingressName string) (*string, error) {
	var ingress *networkv1.Ingress
	err := wait.Poll(time.Millisecond*500, time.Minute*5, func() (bool, error) {
		var err error
		ingress, err = f.KubeClient.NetworkingV1().Ingresses(namespace).Get(f.Ctx, ingressName, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrap(err, fmt.Sprintf("requesting the ingress %v failed", ingressName))
		}
		ingresses := ingress.Status.LoadBalancer.Ingress
		if len(ingresses) != 0 {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return &ingress.Status.LoadBalancer.Ingress[0].IP, nil
}
