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
	var podName string
	flag.Parse()
	podName = flag.Arg(0)

	clientset, err := util.GetClient()
	if err != nil {
		logger.Fatal(err.Error())
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

	_, err = KubectlExec(podName, container.Name, "default", cmdStr)
	if err != nil {
		logger.Errorf("failed to exec cmd %s in pod %s: %++v", cmdStr, podName, err)
		return
	}

	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr
	//err = cmd.Start()
	//if err != nil {
	//	logger.Errorf("failed to exec cmd %s in pod %s: %++v", cmdStr, podName, err)
	//	return
	//}

	watcher, err := clientset.CoreV1().Pods(namespace).Watch(metav1.SingleObject(targetPod.ObjectMeta))
	go func() {
		ctx := context.TODO()
		defer watcher.Stop()
		for {
			select {
			case e := <-watcher.ResultChan():
				{
					if e.Type == watch.Deleted {
						logger.Infof("pod %s deleted", podName)
					}
					if e.Object == nil {
						continue
					}

					pod, ok := e.Object.(*corev1.Pod)
					if !ok {
						continue
					}
					switch pod.Status.Phase {
					case corev1.PodFailed, corev1.PodSucceeded:
						logger.Infof("pod %s status %s", podName, pod.Status.Phase)
					case corev1.PodRunning:
						if pod.GetUID() != podUid {
							logger.Infof("pod %s uid %s, diff from %s", podName, pod.GetUID(), podUid)
						}
					}
					//if podExit {
					//	cmd.Process.Kill()
					//}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	//err = cmd.Wait()
	//if err != nil {
	//	logger.Fatalf("failed to exec cmd  '%s' in pod %s: %++v", cmdStr, podName, err)
	//}
	return
}
