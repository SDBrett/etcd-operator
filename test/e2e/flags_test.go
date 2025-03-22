/*
Copyright 2021 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"log"

	"go.etcd.io/etcd-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var test env.Environment

func TestMain(m *testing.M) {
	// create config from flags (always in TestMain because it calls flag.Parse())
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		log.Fatalf("failed to build envconf from flags: %s", err)
	}
	test = env.NewWithConfig(cfg)

	os.Exit(test.Run(m))
}

func TestNamespace(t *testing.T) {

	feature := features.New("salutation").WithLabel("type", "test").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			ctx, err := envfuncs.CreateNamespace("etcd-test")(ctx, cfg)
			if err != nil {
				log.Printf("failed to create namespace: %s", err)
				return ctx
			}

			r, err := resources.New(cfg.Client().RESTConfig())
			if err != nil {
				t.Fail()
			}
			v1alpha1.AddToScheme(r.GetScheme())
			r.WithNamespace("etcd-test")
			decoder.DecodeEachFile(
				ctx, os.DirFS("./testdata/crs"), "singlecluster.yaml",
				decoder.CreateHandler(r),
			)

			return ctx
		}).
		Assess("Check If Resource created", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			client, err := c.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			r, err := resources.New(c.Client().RESTConfig())
			if err != nil {
				t.Fail()
			}
			r.WithNamespace("etcd-test")

			v1alpha1.AddToScheme(r.GetScheme())
			r.WithNamespace("etcd-test")
			decoder.DecodeEachFile(
				ctx, os.DirFS("./testdata/crs"), "singlecluster.yaml",
				decoder.CreateHandler(r),
			)

			ec := &v1alpha1.EtcdCluster{}
			err = r.Get(ctx, "example", "etcd-test", ec)
			if err != nil {
				t.Fail()
			}

			statefullset := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "etcd-test"},
			}

			err = wait.For(conditions.New(client.Resources()).ResourceMatch(&statefullset, func(object k8s.Object) bool {
				return true
			}), wait.WithTimeout(time.Second*30))
			if err != nil {
				t.Fatal(err)
			}
			return ctx
		}).
		Assess("Check stateful properly configured", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {

			client := c.Client()
			var sts appsv1.StatefulSet
			if err := client.Resources().Get(ctx, "example", "etcd-test", &sts); err != nil {
				t.Fatal(err)
			}
			replicas := *sts.Spec.Replicas
			if replicas != 1 {
				t.Fatalf("Expecting statefulset replicas to be  %d, got %d", 1, replicas)
			}

			expectedArgs := []string{
				"--name=$(POD_NAME)",
				"--listen-peer-urls=http://0.0.0.0:2380",
				"--listen-client-urls=http://0.0.0.0:2379",
				"--initial-advertise-peer-urls=http://$(POD_NAME).example.$(POD_NAMESPACE).svc.cluster.local:2380",
				"--advertise-client-urls=http://$(POD_NAME).example.$(POD_NAMESPACE).svc.cluster.local:2379",
				"--experimental-peer-skip-client-san-verification",
			}

			var etcdContainer corev1.Container
			for c := range sts.Spec.Template.Spec.Containers {
				if sts.Spec.Template.Spec.Containers[c].Name == "etcd" {
					etcdContainer = sts.Spec.Template.Spec.Containers[c]
					break
				}
			}
			if !reflect.DeepEqual(expectedArgs, etcdContainer.Args) {
				t.Fatalf("Expecting args to be %v got %v", expectedArgs, etcdContainer.Args)
			}

			return ctx

		}).Feature()

	_ = test.Test(t, feature)
}
