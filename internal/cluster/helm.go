package cluster

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

type Desired struct {
	Release    string
	Source     string
	Deployment *appsv1.Deployment
	ConfigMaps map[string]*corev1.ConfigMap
}

func LoadDesired(ctx context.Context, cs kubernetes.Interface, dep *appsv1.Deployment) Desired {
	release := dep.Annotations["meta.helm.sh/release-name"]
	if release == "" {
		return Desired{}
	}
	secrets, err := cs.CoreV1().Secrets(dep.Namespace).List(ctx,
		metav1.ListOptions{LabelSelector: "owner=helm,status=deployed,name=" + release})
	if err != nil || len(secrets.Items) == 0 {
		return Desired{}
	}
	manifest, version, err := decodeRelease(secrets.Items[0].Data["release"])
	if err != nil {
		return Desired{}
	}
	desired := Desired{
		Release:    release,
		Source:     fmt.Sprintf("Helm release '%s' (revision %d)", release, version),
		ConfigMaps: map[string]*corev1.ConfigMap{},
	}
	for _, doc := range strings.Split(manifest, "\n---") {
		var meta metav1.TypeMeta
		if yaml.Unmarshal([]byte(doc), &meta) != nil {
			continue
		}
		switch meta.Kind {
		case "Deployment":
			var want appsv1.Deployment
			if yaml.Unmarshal([]byte(doc), &want) == nil && want.Name == dep.Name {
				desired.Deployment = &want
			}
		case "ConfigMap":
			var cm corev1.ConfigMap
			if yaml.Unmarshal([]byte(doc), &cm) == nil {
				desired.ConfigMaps[cm.Name] = &cm
			}
		}
	}
	return desired
}

// Helm stores each release as base64(gzip(json)) inside the Secret's "release" key.
func decodeRelease(data []byte) (manifest string, version int, err error) {
	raw, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return "", 0, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", 0, err
	}
	if raw, err = io.ReadAll(zr); err != nil {
		return "", 0, err
	}
	var rel struct {
		Manifest string `json:"manifest"`
		Version  int    `json:"version"`
	}
	err = json.Unmarshal(raw, &rel)
	return rel.Manifest, rel.Version, err
}
