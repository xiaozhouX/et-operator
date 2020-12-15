package main

import (
	"context"
	"flag"
	"github.com/AliyunContainerService/et-operator/pkg/util"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"os"
	"os/exec"
	"strings"
)

func KubectlExec(podName, containerName, namespace string, cmdStr string) (cmd *exec.Cmd, err error) {
	//binary, err := exec.LookPath("kubectl")
	err = util.ExecWithOptions(util.ExecOptions{
		Command:       []string{"/bin/sh", "-c", cmdStr},
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,

		Stdin:              nil,
		Stdout:             os.Stdout,
		Stderr:             os.Stderr,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})

	return cmd, err
}

func main() {
	flag.Parse()
	podName := flag.Arg(0)

	clientset, err := util.GetClient()
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	targetPod, err := clientset.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get pod %s in %s: %++v", podName, namespace, err)
		return
	}

	podUid := targetPod.GetUID()
	container := targetPod.Spec.Containers[0]

	args := flag.Args()
	cmdStr := strings.Join(args[1:], " ")

	ctx, cancel := context.WithCancel(context.Background())
	if targetPod.Spec.HostPID {
		watcher, err := clientset.CoreV1().Pods(namespace).Watch(metav1.SingleObject(targetPod.ObjectMeta))
		if err != nil {
			logger.Errorf("failed to watch pod %s in %s: %++v", podName, namespace, err)
			return
		}
		defer watcher.Stop()
		go func() {
			for {
				select {
				case e := <-watcher.ResultChan():
					if e.Type == watch.Deleted {
						logger.Infof("pod %s deleted", podName)
						cancel()
					}
					if e.Object == nil {
						continue
					}

					if pod, ok := e.Object.(*corev1.Pod); ok {
						switch pod.Status.Phase {
						case corev1.PodFailed, corev1.PodSucceeded:
							logger.Infof("pod %s status %s", podName, pod.Status.Phase)
							cancel()
						case corev1.PodRunning:
							if pod.GetUID() != podUid {
								logger.Infof("pod %s uid %s, diff from %s", podName, pod.GetUID(), podUid)
							}
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		_, err = KubectlExec(podName, container.Name, namespace, cmdStr)
		if err != nil {
			logger.Errorf("failed to exec cmd %s in pod %s: %++v", cmdStr, podName, err)
			return
		}
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
