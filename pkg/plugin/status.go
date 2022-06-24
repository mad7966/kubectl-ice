package plugin

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	duration "k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var timestampFormat = "2006-01-02 15:04:05"

var statusShort = "List status of each container in a pod"

var statusDescription = ` Prints container status information from pods, current and previous exit code, reason and signal
are shown slong with current ready and running state. Pods and containers can also be selected
by name. If no name is specified the container state of all pods in the current namespace is
shown.

The T column in the table output denotes S for Standard and I for init containers`

var statusExample = `  # List individual container status from pods
  %[1]s status

  # List conttainers status from pods output in JSON format
  %[1]s status -o json

  # List status from all container in a single pod
  %[1]s status my-pod-4jh36

  # List previous container status from a single pod
  %[1]s status -p my-pod-4jh36

  # List status of all containers named web-container searching all 
  # pods in the current namespace
  %[1]s status -c web-container

  # List status of containers called web-container searching all pods in current
  # namespace sorted by container name in descending order (notice the ! charator)
  %[1]s status -c web-container --sort '!CONTAINER'

  # List status of containers called web-container searching all pods in current
  # namespace sorted by pod name in ascending order
  %[1]s status -c web-container --sort PODNAME

  # List container status from all pods where label app equals web
  %[1]s status -l app=web

  # List status from all containers where the pods label app is either web or mail
  %[1]s status -l "app in (web,mail)"`

func Status(cmd *cobra.Command, kubeFlags *genericclioptions.ConfigFlags, args []string) error {
	var columnInfo containerInfomation
	var tblHead []string
	var podname []string
	var showPodName bool = true
	var showPrevious bool
	var labels map[string]map[string]string
	var hideColumns []int

	connect := Connector{}
	if err := connect.LoadConfig(kubeFlags); err != nil {
		return err
	}

	// if a single pod is selected we dont need to show its name
	if len(args) >= 1 {
		podname = args
		if len(podname[0]) >= 1 {
			showPodName = false
		}
	}
	commonFlagList, err := processCommonFlags(cmd)
	if err != nil {
		return err
	}
	connect.Flags = commonFlagList

	podList, err := connect.GetPods(podname)
	if err != nil {
		return err
	}

	if cmd.Flag("previous").Value.String() == "true" {
		showPrevious = true
	}

	if cmd.Flag("tree").Value.String() == "true" {
		columnInfo.treeView = true
	}

	if cmd.Flag("details").Value.String() == "true" {
		commonFlagList.showDetails = true
	}

	if cmd.Flag("node-label").Value.String() != "" {
		columnInfo.labelName = cmd.Flag("node-label").Value.String()
		labels = connect.GetNodeLabels(podList)
	}

	table := Table{}

	tblHead = columnInfo.GetDefaultHead()
	defaultHeaderLen := len(tblHead)
	if columnInfo.treeView {
		//NAMESPACE NODE NAME READY STARTED RESTARTS STATE REASON AGE
		tblHead = append(tblHead, "NAME")
		if commonFlagList.showDetails {
			hideColumns = append(hideColumns, 9)
		} else {
			hideColumns = append(hideColumns, 8)
			hideColumns = append(hideColumns, 10)
		}
	} else {
		//default column ids to hide
		if commonFlagList.showDetails {
			hideColumns = append(hideColumns, 8)
		}
	}

	if showPrevious {
		// STATE REASON EXIT-CODE SIGNAL TIMESTAMP AGE MESSAGE
		hideColumns = append(hideColumns, 0)
		hideColumns = append(hideColumns, 1)
		hideColumns = append(hideColumns, 2)
	}

	if len(hideColumns) == 0 {
		hideColumns = append(hideColumns, 7)
		hideColumns = append(hideColumns, 9)
	}

	tblHead = append(tblHead, "READY", "STARTED", "RESTARTS", "STATE", "REASON", "EXIT-CODE", "SIGNAL", "TIMESTAMP", "AGE", "MESSAGE")
	table.SetHeader(tblHead...)

	if len(commonFlagList.filterList) >= 1 {
		err = table.SetFilter(commonFlagList.filterList)
		if err != nil {
			return err
		}
	}

	commonFlagList.showPodName = showPodName
	columnInfo.SetVisibleColumns(table, commonFlagList)

	// fmt.Println(">>", tblHead)

	for _, id := range hideColumns {
		// fmt.Println("**", id, defaultHeaderLen+id)
		table.HideColumn(defaultHeaderLen + id)
	}

	// do we need to load the node labels
	// something like this maybe??
	// labelList := loadNodeLabels
	// columnInfo.labelName = "appfamily"
	// columnInfo.labelValue = labelList[podname]

	for _, pod := range podList {
		// p := pod.GetOwnerReferences()
		// for i, a := range p {
		// 	fmt.Println("index:", i)
		// 	fmt.Println("** name:", a.Name)
		// 	fmt.Println("** kind:", a.Kind)
		// }

		columnInfo.LoadFromPod(pod)

		//do we need to show the pod line: Pod/foo-6f67dcc579-znb55
		if columnInfo.treeView {
			tblOut := podStatusBuildRow(pod, columnInfo, showPrevious)
			columnInfo.ApplyRow(&table, tblOut)
			// tblFullRow := append(columnInfo.GetDefaultCells(), tblOut...)
			// table.AddRow(tblFullRow...)
		}

		if columnInfo.labelName != "" {
			columnInfo.labelValue = labels[columnInfo.nodeName][columnInfo.podName]
		}

		//now show the container line
		columnInfo.containerType = "S"
		for _, container := range pod.Status.ContainerStatuses {
			// should the container be processed
			if skipContainerName(commonFlagList, container.Name) {
				continue
			}
			columnInfo.containerName = container.Name
			tblOut := statusBuildRow(container, columnInfo, showPrevious)
			columnInfo.ApplyRow(&table, tblOut)
			// tblFullRow := append(columnInfo.GetDefaultCells(), tblOut...)
			// table.AddRow(tblFullRow...)
		}

		columnInfo.containerType = "I"
		for _, container := range pod.Status.InitContainerStatuses {
			// should the container be processed
			if skipContainerName(commonFlagList, container.Name) {
				continue
			}
			columnInfo.containerName = container.Name
			tblOut := statusBuildRow(container, columnInfo, showPrevious)
			columnInfo.ApplyRow(&table, tblOut)
			// tblFullRow := append(columnInfo.GetDefaultCells(), tblOut...)
			// table.AddRow(tblFullRow...)
		}

		columnInfo.containerType = "E"
		for _, container := range pod.Status.EphemeralContainerStatuses {
			// should the container be processed
			if skipContainerName(commonFlagList, container.Name) {
				continue
			}
			columnInfo.containerName = container.Name
			tblOut := statusBuildRow(container, columnInfo, showPrevious)
			columnInfo.ApplyRow(&table, tblOut)
			// tblFullRow := append(columnInfo.GetDefaultCells(), tblOut...)
			// table.AddRow(tblFullRow...)
		}
	}

	// sorting by column breaks the tree view also previous is not valid so we sliently skip those actions
	if !columnInfo.treeView {
		if err := table.SortByNames(commonFlagList.sortList...); err != nil {
			return err
		}

		if !showPrevious { // restart count dosent show up when using previous flag
			// do we need to find the outliers, we have enough data to compute a range
			if commonFlagList.showOddities {
				row2Remove, err := table.ListOutOfRange(6, table.GetRows()) //3 = restarts column
				if err != nil {
					return err
				}
				table.HideRows(row2Remove)
			}
		}
	}

	outputTableAs(table, commonFlagList.outputAs)
	return nil

}

func podStatusBuildRow(pod v1.Pod, info containerInfomation, showPrevious bool) []Cell {
	var age string
	var timestamp string

	phase := string(pod.Status.Phase)
	if pod.Status.StartTime != nil {
		starttime := pod.Status.StartTime.Time
		timestamp = starttime.Format(timestampFormat)
		rawAge := time.Since(starttime)
		age = duration.HumanDuration(rawAge)
	}

	return []Cell{
		NewCellText(fmt.Sprint("Pod/", info.podName)), //name
		NewCellText(""),                       //ready
		NewCellText(""),                       //started
		NewCellInt("0", 0),                    //restarts
		NewCellText(strings.TrimSpace(phase)), //state
		NewCellText(pod.Status.Reason),        //reason
		NewCellText(""),                       //exit-code
		NewCellText(""),                       //signal
		NewCellText(timestamp),                //timestamp
		NewCellText(age),                      //age
		NewCellText(""),                       //message
	}
}

func statusBuildRow(container v1.ContainerStatus, info containerInfomation, showPrevious bool) []Cell {
	var cellList []Cell
	var reason string
	var exitCode string
	var signal string
	var message string
	var startedAt string
	var startTime time.Time
	var skipAgeCalculation bool
	var started string
	var strState string
	var age string
	var state v1.ContainerState
	var rawExitCode, rawSignal, rawRestarts int64

	// fmt.Println("F:statusBuildRow:Name=", container.Name)

	if showPrevious {
		state = container.LastTerminationState
	} else {
		state = container.State
	}

	if state.Waiting != nil {
		strState = "Waiting"
		reason = state.Waiting.Reason
		message = state.Waiting.Message
		// waiting state dosent have a start time so we skip setting the age variable, used further down
		skipAgeCalculation = true
	}

	if state.Terminated != nil {
		strState = "Terminated"
		exitCode = fmt.Sprintf("%d", state.Terminated.ExitCode)
		rawExitCode = int64(state.Terminated.ExitCode)
		signal = fmt.Sprintf("%d", state.Terminated.Signal)
		rawSignal = int64(state.Terminated.Signal)
		startTime = state.Terminated.StartedAt.Time
		startedAt = state.Terminated.StartedAt.Format(timestampFormat)
		reason = state.Terminated.Reason
		message = state.Terminated.Message
	}

	if state.Running != nil {
		strState = "Running"
		startedAt = state.Running.StartedAt.Format(timestampFormat)
		startTime = state.Running.StartedAt.Time
	}

	if container.Started != nil {
		started = fmt.Sprintf("%t", *container.Started)
	}

	ready := fmt.Sprintf("%t", container.Ready)
	restarts := fmt.Sprintf("%d", container.RestartCount)
	rawRestarts = int64(container.RestartCount)
	// remove pod and container name from the message string
	message = trimStatusMessage(message, info.podName, info.containerName)

	//we can only show the age if we have a start time some states dont have said starttime so we have to skip them
	if skipAgeCalculation {
		age = ""
	} else {
		rawAge := time.Since(startTime)
		age = duration.HumanDuration(rawAge)
	}

	if info.treeView {
		var namePrefix string
		if info.containerType == "S" {
			namePrefix = "Container/"
		}
		if info.containerType == "I" {
			namePrefix = "InitContainer/"
		}
		if info.containerType == "E" {
			namePrefix = "EphemeralContainer/"
		}

		cellList = append(cellList,
			NewCellText(fmt.Sprint("└─", namePrefix, info.containerName)),
		)
	}

	// READY STARTED RESTARTS STATE REASON EXIT-CODE SIGNAL TIMESTAMP AGE MESSAGE
	cellList = append(cellList,
		NewCellText(ready),
		NewCellText(started),
		NewCellInt(restarts, rawRestarts),
		NewCellText(strState),
		NewCellText(reason),
		NewCellInt(exitCode, rawExitCode),
		NewCellInt(signal, rawSignal),
		NewCellText(startedAt),
		NewCellText(age),
		NewCellText(message),
	)

	return cellList
}

// Removes the pod name and container name from the status message as its already in the output table
func trimStatusMessage(message string, podName string, containerName string) string {

	if len(message) <= 0 {
		return ""
	}
	if len(podName) <= 0 {
		return ""
	}
	if len(containerName) <= 0 {
		return ""
	}

	newMessage := ""
	strArray := strings.Split(message, " ")
	for _, v := range strArray {
		if "container="+containerName == v {
			continue
		}
		if strings.HasPrefix(v, "pod="+podName+"_") {
			continue
		}
		newMessage += " " + v
	}
	return strings.TrimSpace(newMessage)
}
