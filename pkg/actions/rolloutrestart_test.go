package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func rolloutrestartTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func testDeployment(ns, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(replicas),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          replicas,
			ReadyReplicas:     replicas,
			AvailableReplicas: replicas,
			UpdatedReplicas:   replicas,
		},
	}
}

func TestRolloutRestart(t *testing.T) {
	action := &rolloutRestart{}

	t.Run("When namespace param is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "name": "my-deploy"},
			KubeClient: fake.NewClientset(),
			Logger:     rolloutrestartTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing namespace")
		}
	})

	t.Run("When name param is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "namespace": "default"},
			KubeClient: fake.NewClientset(),
			Logger:     rolloutrestartTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing name")
		}
	})

	t.Run("When deployment does not exist it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "namespace": "default", "name": "no-such-deploy"},
			KubeClient: fake.NewClientset(),
			Logger:     rolloutrestartTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for non-existent deployment")
		}
	})

	t.Run("When deployment has zero replicas it should refuse restart", func(t *testing.T) {
		client := fake.NewClientset(testDeployment("default", "scaled-down", 0))
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "namespace": "default", "name": "scaled-down"},
			KubeClient: client,
			Logger:     rolloutrestartTestLogger(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected validation error for zero-replica deployment")
		}
	})

	t.Run("When deployment is valid it should pass validation", func(t *testing.T) {
		client := fake.NewClientset(testDeployment("default", "good-deploy", 3))
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "namespace": "default", "name": "good-deploy"},
			KubeClient: client,
			Logger:     rolloutrestartTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("When resource type is unsupported it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "cronjob", "namespace": "default", "name": "my-job"},
			KubeClient: fake.NewClientset(),
			Logger:     rolloutrestartTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for unsupported resource type")
		}
	})

	t.Run("When restarting a valid deployment it should patch the restart annotation", func(t *testing.T) {
		client := fake.NewClientset(testDeployment("default", "restart-me", 2))
		params := &ExecutionParams{
			Params:     map[string]string{"resource": "deployment", "namespace": "default", "name": "restart-me"},
			KubeClient: client,
			Logger:     rolloutrestartTestLogger(),
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := action.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatal("expected success")
		}
		if len(result.AffectedResources) != 1 {
			t.Fatalf("expected 1 affected resource, got %d", len(result.AffectedResources))
		}
		if result.AffectedResources[0].Action != "restarted" {
			t.Errorf("expected action 'restarted', got %q", result.AffectedResources[0].Action)
		}

		deploy, err := client.AppsV1().Deployments("default").Get(ctx, "restart-me", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get deployment: %v", err)
		}

		ann := deploy.Spec.Template.Annotations
		if ann == nil {
			t.Fatal("expected restart annotation to be set")
		}
		if _, ok := ann[restartAnnotation]; !ok {
			t.Error("expected restart annotation key to be present")
		}
	})
}

func TestRolloutRestartMetadata(t *testing.T) {
	action := &rolloutRestart{}
	meta := action.Metadata()

	t.Run("When checking metadata it should have correct properties", func(t *testing.T) {
		if meta.Name != "rollout_restart" {
			t.Errorf("expected name 'rollout_restart', got %q", meta.Name)
		}
		if meta.Type != "write" {
			t.Errorf("expected type 'write', got %q", meta.Type)
		}
		if meta.WriteCooldownSeconds != 300 {
			t.Errorf("expected cooldown 300, got %d", meta.WriteCooldownSeconds)
		}
		if meta.DryRunAction != "get_resource" {
			t.Errorf("expected dry run action 'get_resource', got %q", meta.DryRunAction)
		}
	})
}

func TestIsDeploymentRolloutComplete(t *testing.T) {
	t.Run("When all replicas are updated and ready it should return true", func(t *testing.T) {
		d := testDeployment("default", "complete", 3)
		d.Status.ObservedGeneration = d.Generation
		if !isDeploymentRolloutComplete(d) {
			t.Error("expected rollout to be complete")
		}
	})

	t.Run("When updated replicas lag behind it should return false", func(t *testing.T) {
		d := testDeployment("default", "incomplete", 3)
		d.Status.ObservedGeneration = d.Generation
		d.Status.UpdatedReplicas = 1
		if isDeploymentRolloutComplete(d) {
			t.Error("expected rollout to be incomplete")
		}
	})

	t.Run("When observed generation is behind it should return false", func(t *testing.T) {
		d := testDeployment("default", "stale", 3)
		d.Generation = 2
		d.Status.ObservedGeneration = 1
		if isDeploymentRolloutComplete(d) {
			t.Error("expected rollout to be incomplete when generation is behind")
		}
	})
}
