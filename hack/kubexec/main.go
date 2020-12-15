package main

import (
	"context"
	"flag"
	"github.com/AliyunContainerService/et-operator/pkg/util"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"os"
	"os/exec"
	"strings"
)

func main() {
	flag.Parse()
	podName := flag.Arg(0)

	clientset, err := util.GetClient()
	if err != nil {
		logger.Errorf(err.Error())
		os.Exit(255)
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	targetPod, err := clientset.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get pod %s in %s: %++v", podName, namespace, err)
		os.Exit(255)
	}
	container := targetPod.Spec.Containers[0]

	podExitCtx, podWatchCancel, err := watchPodRunning(clientset, targetPod)
	if err != nil {
		logger.Errorf("failed to get pod %s in %s: %++v", podName, namespace, err)
		os.Exit(255)
	}
	defer podWatchCancel()

	kubexecCtx, kubexecFinish := context.WithCancel(context.Background())
	defer kubexecFinish()
	go func() {
		args := flag.Args()
		cmdStr := strings.Join(args[1:], " ")
		_, err = kubectlExec(podName, container.Name, namespace, cmdStr)
		if err != nil {
			logger.Errorf("failed to exec cmd %s in pod %s: %++v", cmdStr, podName, err)
			os.Exit(255)
		}
		kubexecFinish()
	}()

	for {
		select {
		case <-podExitCtx.Done():
			os.Exit(255)
			logger.Info("pod exit")
		case <-kubexecCtx.Done():
			logger.Info("pod exec finish")
			return
		}
	}
}

func kubectlExec(podName, containerName, namespace string, cmdStr string) (cmd *exec.Cmd, err error) {
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

func watchPodRunning(clientset *kubernetes.Clientset, targetPod *corev1.Pod) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	podName := targetPod.Name
	namespace := targetPod.Namespace
	podUid := targetPod.GetUID()
	if targetPod.Spec.HostPID {
		watcher, err := clientset.CoreV1().Pods(namespace).Watch(metav1.SingleObject(targetPod.ObjectMeta))
		if err != nil {
			logger.Errorf("failed to watch pod %s in %s: %++v", podName, namespace, err)
			return ctx, cancel, err
		}
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
						case corev1.PodFailed, corev1.PodSucceeded, corev1.PodUnknown:
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
		return ctx, func() {
			cancel()
			watcher.Stop()
		}, nil
	}
	return ctx, cancel, nil
}
