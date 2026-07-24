package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"context"

	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- helpers ---

func makePullSecretJSON(auths map[string]map[string]string) []byte {
	ps := map[string]map[string]map[string]string{"auths": auths}
	b, err := json.Marshal(ps)
	if err != nil {
		panic(fmt.Sprintf("makePullSecretJSON: %v", err))
	}
	return b
}

// --- ValidateRequiredAuths ---

func buildAuthMap(registries ...string) map[string]*amv1.AccessTokenAuth {
	m := make(map[string]*amv1.AccessTokenAuth, len(registries))
	for _, r := range registries {
		auth, err := amv1.NewAccessTokenAuth().Auth("tok").Email("e@e").Build()
		if err != nil {
			panic(fmt.Sprintf("buildAuthMap: %v", err))
		}
		m[r] = auth
	}
	return m
}

func TestValidateRequiredAuths_AllPresent(t *testing.T) {
	auths := buildAuthMap(RequiredPullSecretAuths...)
	missing := ValidateRequiredAuths(auths)
	if len(missing) != 0 {
		t.Fatalf("expected 0 missing, got %v", missing)
	}
}

func TestValidateRequiredAuths_SomeMissing(t *testing.T) {
	auths := buildAuthMap("quay.io")
	missing := ValidateRequiredAuths(auths)
	if len(missing) != 3 {
		t.Fatalf("expected 3 missing, got %d: %v", len(missing), missing)
	}
}

func TestValidateRequiredAuths_Empty(t *testing.T) {
	missing := ValidateRequiredAuths(map[string]*amv1.AccessTokenAuth{})
	if len(missing) != 4 {
		t.Fatalf("expected 4 missing, got %d", len(missing))
	}
}

// --- CompareThreeWay ---

func TestCompareThreeWay_AllInSync(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io":            {Auth: "tok1", Email: "a@b"},
		"registry.redhat.io": {Auth: "tok2", Email: "a@b"},
	}
	hive := makePullSecretJSON(map[string]map[string]string{
		"quay.io":            {"auth": "tok1", "email": "a@b"},
		"registry.redhat.io": {"auth": "tok2", "email": "a@b"},
	})
	target := makePullSecretJSON(map[string]map[string]string{
		"quay.io":            {"auth": "tok1", "email": "a@b"},
		"registry.redhat.io": {"auth": "tok2", "email": "a@b"},
	})

	result, err := CompareThreeWay(ocm, hive, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllInSync {
		t.Error("expected AllInSync=true")
	}
	if result.HiveNeedsUpdate {
		t.Error("expected HiveNeedsUpdate=false")
	}
	if result.TargetNeedsSync {
		t.Error("expected TargetNeedsSync=false")
	}
	if len(result.Auths) != 2 {
		t.Errorf("expected 2 auth states, got %d", len(result.Auths))
	}
}

func TestCompareThreeWay_TargetDiffers(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "tok1", Email: "a@b"},
	}
	hive := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok1", "email": "a@b"},
	})
	target := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok1", "email": "tampered@bad.com"},
	})

	result, err := CompareThreeWay(ocm, hive, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllInSync {
		t.Error("expected AllInSync=false")
	}
	if !result.TargetNeedsSync {
		t.Error("expected TargetNeedsSync=true")
	}
	if result.HiveNeedsUpdate {
		t.Error("expected HiveNeedsUpdate=false (hive matches OCM)")
	}
}

func TestCompareThreeWay_HiveDiffers(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "newtok", Email: "a@b"},
	}
	hive := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "oldtok", "email": "a@b"},
	})
	target := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "oldtok", "email": "a@b"},
	})

	result, err := CompareThreeWay(ocm, hive, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllInSync {
		t.Error("expected AllInSync=false")
	}
	if !result.HiveNeedsUpdate {
		t.Error("expected HiveNeedsUpdate=true")
	}
	if !result.TargetNeedsSync {
		t.Error("expected TargetNeedsSync=true")
	}
}

func TestCompareThreeWay_NilHive(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "tok", Email: "a@b"},
	}
	target := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok", "email": "a@b"},
	})

	result, err := CompareThreeWay(ocm, nil, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AllInSync {
		t.Error("expected AllInSync=false when hive is nil")
	}
	if !result.HiveNeedsUpdate {
		t.Error("expected HiveNeedsUpdate=true when hive is nil")
	}
	if result.TargetNeedsSync {
		t.Error("expected TargetNeedsSync=false since target matches OCM")
	}
}

func TestCompareThreeWay_ExtraRegistryInTarget(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "tok", Email: "a@b"},
	}
	hive := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok", "email": "a@b"},
	})
	target := makePullSecretJSON(map[string]map[string]string{
		"quay.io":            {"auth": "tok", "email": "a@b"},
		"custom.registry.io": {"auth": "custom", "email": "c@d"},
	})

	result, err := CompareThreeWay(ocm, hive, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AllInSync {
		t.Error("expected AllInSync=true — extra registries in target are not in OCM scope")
	}
	if len(result.Auths) != 1 {
		t.Errorf("expected 1 auth state (only quay.io), got %d", len(result.Auths))
	}
}

func TestCompareThreeWay_InvalidHiveJSON(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "tok", Email: "a@b"},
	}
	_, err := CompareThreeWay(ocm, []byte("not json"), nil)
	if err == nil {
		t.Error("expected error for invalid hive JSON")
	}
}

func TestCompareThreeWay_InvalidTargetJSON(t *testing.T) {
	ocm := map[string]SimpleAuth{
		"quay.io": {Auth: "tok", Email: "a@b"},
	}
	_, err := CompareThreeWay(ocm, nil, []byte("{corrupt"))
	if err == nil {
		t.Error("expected error for invalid target JSON")
	}
}

// --- ExtractRegistryAuth ---

func TestExtractRegistryAuth_Valid(t *testing.T) {
	ps := makePullSecretJSON(map[string]map[string]string{
		"cloud.openshift.com": {"auth": "dGVzdHRva2Vu", "email": "a@b"},
	})
	auth, err := ExtractRegistryAuth(ps, "cloud.openshift.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth != "dGVzdHRva2Vu" {
		t.Errorf("expected auth=dGVzdHRva2Vu, got %s", auth)
	}
}

func TestExtractRegistryAuth_MissingRegistry(t *testing.T) {
	ps := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok", "email": "a@b"},
	})
	_, err := ExtractRegistryAuth(ps, "cloud.openshift.com")
	if err == nil {
		t.Error("expected error for missing registry")
	}
}

func TestExtractRegistryAuth_InvalidJSON(t *testing.T) {
	_, err := ExtractRegistryAuth([]byte("not json"), "cloud.openshift.com")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExtractRegistryAuth_EmptyAuths(t *testing.T) {
	ps := []byte(`{"auths":{}}`)
	_, err := ExtractRegistryAuth(ps, "cloud.openshift.com")
	if err == nil {
		t.Error("expected error for empty auths")
	}
}

// --- MergePullSecretAuths ---

func TestMergePullSecretAuths_AddNew(t *testing.T) {
	existing := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok1", "email": "a@b"},
	})
	incoming := makePullSecretJSON(map[string]map[string]string{
		"registry.redhat.io": {"auth": "tok2", "email": "a@b"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct {
			Auth  string `json:"auth"`
			Email string `json:"email"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if len(parsed.Auths) != 2 {
		t.Fatalf("expected 2 auths, got %d", len(parsed.Auths))
	}
	if parsed.Auths["quay.io"].Auth != "tok1" {
		t.Error("existing auth was overwritten")
	}
	if parsed.Auths["registry.redhat.io"].Auth != "tok2" {
		t.Error("new auth not added")
	}
}

func TestMergePullSecretAuths_OverwriteExisting(t *testing.T) {
	existing := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "oldtok", "email": "old@e"},
	})
	incoming := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "newtok", "email": "new@e"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct {
			Auth  string `json:"auth"`
			Email string `json:"email"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if parsed.Auths["quay.io"].Auth != "newtok" {
		t.Error("incoming auth did not overwrite existing")
	}
	if parsed.Auths["quay.io"].Email != "new@e" {
		t.Error("incoming email did not overwrite existing")
	}
}

func TestMergePullSecretAuths_NeverDeletes(t *testing.T) {
	existing := makePullSecretJSON(map[string]map[string]string{
		"quay.io":                     {"auth": "tok1", "email": "a@b"},
		"registry.redhat.io":          {"auth": "tok2", "email": "a@b"},
		"registry.connect.redhat.com": {"auth": "tok3", "email": "a@b"},
	})
	incoming := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "newtok", "email": "a@b"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct{} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if len(parsed.Auths) != 3 {
		t.Fatalf("merge deleted auths: expected 3, got %d", len(parsed.Auths))
	}
}

func TestMergePullSecretAuths_InvalidJSON(t *testing.T) {
	_, err := MergePullSecretAuths([]byte("not json"), makePullSecretJSON(map[string]map[string]string{}))
	if err == nil {
		t.Error("expected error for invalid existing JSON")
	}

	_, err = MergePullSecretAuths(makePullSecretJSON(map[string]map[string]string{}), []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid incoming JSON")
	}
}

func TestMergePullSecretAuths_NilAuthsMap(t *testing.T) {
	existing := []byte(`{"auths":null}`)
	incoming := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok1", "email": "a@b"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct{} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if len(parsed.Auths) != 1 {
		t.Fatalf("expected 1 auth, got %d", len(parsed.Auths))
	}
}

func TestMergePullSecretAuths_EmptyAuthSkipped(t *testing.T) {
	existing := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "validtok", "email": "a@b"},
	})
	incoming := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "", "email": "a@b"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if parsed.Auths["quay.io"].Auth != "validtok" {
		t.Errorf("empty auth should not overwrite valid auth, got %q", parsed.Auths["quay.io"].Auth)
	}
}

func TestMergePullSecretAuths_EmptyExisting(t *testing.T) {
	existing := []byte(`{}`)
	incoming := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "tok1", "email": "a@b"},
	})

	merged, err := MergePullSecretAuths(existing, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Auths map[string]struct{} `json:"auths"`
	}
	if err := json.Unmarshal(merged, &parsed); err != nil {
		t.Fatalf("failed to parse merged: %v", err)
	}
	if len(parsed.Auths) != 1 {
		t.Fatalf("expected 1 auth, got %d", len(parsed.Auths))
	}
}

// --- extractPullSecretAuth ---

func TestExtractPullSecretAuth_Valid(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": makePullSecretJSON(map[string]map[string]string{
				"quay.io": {"auth": "tok1", "email": "a@b"},
			}),
		},
	}

	entry, err := extractPullSecretAuth("quay.io", secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.auth != "tok1" {
		t.Errorf("expected auth=tok1, got %s", entry.auth)
	}
	if entry.email != "a@b" {
		t.Errorf("expected email=a@b, got %s", entry.email)
	}
}

func TestExtractPullSecretAuth_MissingRegistry(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": makePullSecretJSON(map[string]map[string]string{
				"quay.io": {"auth": "tok1", "email": "a@b"},
			}),
		},
	}

	_, err := extractPullSecretAuth("registry.redhat.io", secret)
	if err == nil {
		t.Error("expected error for missing registry")
	}
}

func TestExtractPullSecretAuth_MissingDockerConfig(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{}}
	_, err := extractPullSecretAuth("quay.io", secret)
	if err == nil {
		t.Error("expected error for missing .dockerconfigjson")
	}
}

func TestExtractPullSecretAuth_InvalidJSON(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte("not json"),
		},
	}
	_, err := extractPullSecretAuth("quay.io", secret)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- updateManifestWorkPayloads ---

func TestUpdateManifestWorkPayloads_UpdatesSecretAndHC(t *testing.T) {
	secret := &corev1.Secret{}
	secret.APIVersion = "v1"
	secret.Kind = "Secret"
	secret.Name = "test-ps-secret-abc"
	secret.Namespace = "clusters"
	secret.Data = map[string][]byte{
		".dockerconfigjson": makePullSecretJSON(map[string]map[string]string{
			"quay.io": {"auth": "oldtok", "email": "old@e"},
		}),
	}
	secretJSON, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("failed to marshal secret: %v", err)
	}

	hc := &hypershiftv1beta1.HostedCluster{}
	hc.APIVersion = "hypershift.openshift.io/v1beta1"
	hc.Kind = "HostedCluster"
	hc.Name = "test-cluster"
	hc.Spec.PullSecret.Name = "test-ps-secret-abc"
	hcJSON, err := json.Marshal(hc)
	if err != nil {
		t.Fatalf("failed to marshal HC: %v", err)
	}

	mw := &workv1.ManifestWork{
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: secretJSON}},
					{RawExtension: runtime.RawExtension{Raw: hcJSON}},
				},
			},
		},
	}

	newPS := makePullSecretJSON(map[string]map[string]string{
		"quay.io":            {"auth": "newtok", "email": "new@e"},
		"registry.redhat.io": {"auth": "tok2", "email": "new@e"},
	})

	err = updateManifestWorkPayloads(mw, "test-ps-secret", "test-ps-secret-xyz", newPS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the secret was updated
	var updatedSecret corev1.Secret
	if err := json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, &updatedSecret); err != nil {
		t.Fatalf("failed to parse updated secret: %v", err)
	}
	if updatedSecret.Name != "test-ps-secret-xyz" {
		t.Errorf("secret name not updated: got %s", updatedSecret.Name)
	}
	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(updatedSecret.Data[".dockerconfigjson"], &parsed); err != nil {
		t.Fatalf("failed to parse updated secret data: %v", err)
	}
	if parsed.Auths["quay.io"].Auth != "newtok" {
		t.Error("secret auth not updated")
	}
	if _, ok := parsed.Auths["registry.redhat.io"]; !ok {
		t.Error("new registry not added to secret")
	}

	// Verify the HostedCluster pullSecret ref was updated
	var updatedHC hypershiftv1beta1.HostedCluster
	if err := json.Unmarshal(mw.Spec.Workload.Manifests[1].Raw, &updatedHC); err != nil {
		t.Fatalf("failed to parse updated HC: %v", err)
	}
	if updatedHC.Spec.PullSecret.Name != "test-ps-secret-xyz" {
		t.Errorf("HC pullSecret name not updated: got %s", updatedHC.Spec.PullSecret.Name)
	}
}

func TestUpdateManifestWorkPayloads_SkipsNonMatchingSecret(t *testing.T) {
	secret := &corev1.Secret{}
	secret.APIVersion = "v1"
	secret.Kind = "Secret"
	secret.Name = "other-secret"
	secret.Namespace = "clusters"
	secret.Data = map[string][]byte{"key": []byte("value")}
	secretJSON, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("failed to marshal secret: %v", err)
	}

	mw := &workv1.ManifestWork{
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: secretJSON}},
				},
			},
		},
	}

	err = updateManifestWorkPayloads(mw, "test-ps-secret", "test-ps-secret-new", []byte(`{"auths":{}}`))
	if err == nil {
		t.Fatal("expected error when no Secret matches prefix")
	}

	var unchanged corev1.Secret
	if err := json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, &unchanged); err != nil {
		t.Fatalf("failed to parse unchanged secret: %v", err)
	}
	if unchanged.Name != "other-secret" {
		t.Error("non-matching secret was modified")
	}
}

func TestUpdateManifestWorkPayloads_SecretWithoutHC(t *testing.T) {
	secret := &corev1.Secret{}
	secret.APIVersion = "v1"
	secret.Kind = "Secret"
	secret.Name = "test-ps-secret-abc"
	secret.Data = map[string][]byte{
		".dockerconfigjson": makePullSecretJSON(map[string]map[string]string{
			"quay.io": {"auth": "oldtok", "email": "old@e"},
		}),
	}
	secretJSON, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("failed to marshal secret: %v", err)
	}

	mw := &workv1.ManifestWork{
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{RawExtension: runtime.RawExtension{Raw: secretJSON}},
				},
			},
		},
	}

	newPS := makePullSecretJSON(map[string]map[string]string{
		"quay.io": {"auth": "newtok", "email": "new@e"},
	})

	err = updateManifestWorkPayloads(mw, "test-ps-secret", "test-ps-secret-xyz", newPS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated corev1.Secret
	if err := json.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, &updated); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if updated.Name != "test-ps-secret-xyz" {
		t.Errorf("secret name not updated: got %s", updated.Name)
	}
}

// --- PullSecretOp ---

func TestPullSecretOp_Section(t *testing.T) {
	var buf bytes.Buffer
	op := NewPullSecretOp(false, logrus.New(), &buf)
	op.Section(1, "Test Step", "Line one", "Line two")

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("Step 1:")) {
		t.Error("section missing step number")
	}
	if !bytes.Contains([]byte(out), []byte("Test Step")) {
		t.Error("section missing title")
	}
	if !bytes.Contains([]byte(out), []byte("Line one")) {
		t.Error("section missing description line")
	}
}

func TestPullSecretOp_DryRunPrefixes(t *testing.T) {
	var buf bytes.Buffer
	op := NewPullSecretOp(true, logrus.New(), &buf)

	op.OK("test ok")
	if !bytes.Contains(buf.Bytes(), []byte("[Dry Run]")) {
		t.Error("dry-run OK missing prefix")
	}
	if !bytes.Contains(buf.Bytes(), []byte("[OK]")) {
		t.Error("OK missing [OK] marker")
	}

	buf.Reset()
	op.Fail("test fail %s", "reason")
	if !bytes.Contains(buf.Bytes(), []byte("[Dry Run]")) {
		t.Error("dry-run Fail missing prefix")
	}
	if !bytes.Contains(buf.Bytes(), []byte("[FAIL]")) {
		t.Error("Fail missing [FAIL] marker")
	}
	buf.Reset()
	op.Would("do something")
	if !bytes.Contains(buf.Bytes(), []byte("Would:")) {
		t.Error("Would missing Would: prefix")
	}
}

func TestPullSecretOp_LiveModePrefixes(t *testing.T) {
	var buf bytes.Buffer
	op := NewPullSecretOp(false, logrus.New(), &buf)

	op.OK("live ok")
	if bytes.Contains(buf.Bytes(), []byte("[Dry Run]")) {
		t.Error("live mode should not have [Dry Run] prefix")
	}
	if !bytes.Contains(buf.Bytes(), []byte("[OK]")) {
		t.Error("live OK missing [OK] marker")
	}
}

func TestPullSecretOp_FailTracking(t *testing.T) {
	var buf bytes.Buffer
	op := NewPullSecretOp(false, logrus.New(), &buf)

	op.OK("good thing")
	if len(op.Failures) != 0 {
		t.Error("OK should not add failures")
	}

	op.Fail("bad thing: %s", "reason")
	if len(op.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(op.Failures))
	}
	if op.Failures[0] != "bad thing: reason" {
		t.Errorf("failure message wrong: %s", op.Failures[0])
	}

	op.Fail("another fail")
	if len(op.Failures) != 2 {
		t.Fatalf("expected 2 failures, got %d", len(op.Failures))
	}
}

// --- syncStatus ---

func TestSyncStatus(t *testing.T) {
	tests := []struct {
		name     string
		present  bool
		matches  bool
		expected string
	}{
		{"present and matches", true, true, "match"},
		{"present but differs", true, false, "DIFFERS"},
		{"not present", false, false, "missing"},
		{"not present but matches (edge)", false, true, "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syncStatus(tt.present, tt.matches)
			if got != tt.expected {
				t.Errorf("syncStatus(%v, %v) = %q, want %q", tt.present, tt.matches, got, tt.expected)
			}
		})
	}
}

// --- pullSecretScheme for fake client tests ---

func pullSecretScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1: %v", err)
	}
	if err := hiveapiv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add hiveapiv1: %v", err)
	}
	if err := hiveinternalv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add hiveinternalv1alpha1: %v", err)
	}
	return s
}

// --- CheckExistingSyncSets ---

func TestCheckExistingSyncSets_NoExisting(t *testing.T) {
	kubeCli := fake.NewClientBuilder().WithScheme(pullSecretScheme(t)).Build()
	var buf bytes.Buffer
	err := CheckExistingSyncSets(context.Background(), "uhc-test-ns", kubeCli, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckExistingSyncSets_DetectsOurSyncSet(t *testing.T) {
	ss := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SyncSetName,
			Namespace: "uhc-test-ns",
		},
	}
	kubeCli := fake.NewClientBuilder().WithScheme(pullSecretScheme(t)).WithRuntimeObjects(ss).Build()
	var buf bytes.Buffer

	// Would prompt for input — since stdin is closed, ReadString returns EOF,
	// response is empty, doesn't match "y"/"yes", returns abort error
	err := CheckExistingSyncSets(context.Background(), "uhc-test-ns", kubeCli, &buf)
	if err == nil {
		t.Fatal("expected abort error when SyncSet exists and user doesn't confirm")
	}
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Existing SyncSet")) {
		t.Error("expected warning about existing SyncSet")
	}
	if !bytes.Contains([]byte(output), []byte("No changes have been made")) {
		t.Error("expected 'no changes' safety message")
	}
}

func TestCheckExistingSyncSets_DetectsTransferOwnerSyncSet(t *testing.T) {
	ss := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret-replacement",
			Namespace: "uhc-test-ns",
		},
	}
	kubeCli := fake.NewClientBuilder().WithScheme(pullSecretScheme(t)).WithRuntimeObjects(ss).Build()
	var buf bytes.Buffer

	err := CheckExistingSyncSets(context.Background(), "uhc-test-ns", kubeCli, &buf)
	if err == nil {
		t.Fatal("expected abort error when transfer-owner SyncSet exists")
	}
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("transfer-owner")) {
		t.Error("expected message about transfer-owner SyncSet")
	}
}

// --- FindHiveNamespace ---

func TestFindHiveNamespace_FoundByLabel(t *testing.T) {
	cd := &hiveapiv1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cd",
			Namespace: "uhc-production-abc123",
			Labels:    map[string]string{"api.openshift.com/id": "abc123"},
		},
	}
	kubeCli := fake.NewClientBuilder().WithScheme(pullSecretScheme(t)).WithRuntimeObjects(cd).Build()

	info, err := FindHiveNamespace(context.Background(), kubeCli, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Namespace != "uhc-production-abc123" {
		t.Errorf("expected namespace uhc-production-abc123, got %s", info.Namespace)
	}
	if info.ClusterDeploymentName != "test-cd" {
		t.Errorf("expected CD name test-cd, got %s", info.ClusterDeploymentName)
	}
}

func TestFindHiveNamespace_NotFound(t *testing.T) {
	kubeCli := fake.NewClientBuilder().WithScheme(pullSecretScheme(t)).Build()

	_, err := FindHiveNamespace(context.Background(), kubeCli, "nonexistent")
	if err == nil {
		t.Fatal("expected error when no ClusterDeployment found")
	}
}
