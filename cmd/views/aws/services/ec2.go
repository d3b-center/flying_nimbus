package aws

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	awsService "flying_nimbus/cmd/services/aws"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type EC2View struct {
	Layout        tview.Primitive
	pages         *tview.Pages
	app           *tview.Application
	instanceList  *tview.List
	detailsPanel  *tview.TextView
	ec2Service    *awsService.EC2Service
	ec2SSMService *awsService.SSMService
	iamService    *awsService.IAMService
	instances     []awsService.EC2Instance
}

func NewEC2View(pages *tview.Pages, app *tview.Application) *EC2View {
	ev := &EC2View{
		pages: pages,
		app:   app,
	}

	// Initialize AWS service
	ctx := context.Background()
	var err error
	ev.ec2Service, err = awsService.NewEC2Service(ctx)
	if err != nil {
		ev.ec2Service = nil
	}
	ev.ec2SSMService, err = awsService.NewSSMService(ctx)
	if err != nil {
		ev.ec2SSMService = nil
	}
	ev.iamService, err = awsService.NewIAMService(ctx)
	if err != nil {
		ev.iamService = nil
	}
	if err != nil {
		// Handle error - show error view
		ev.buildErrorLayout(err)
		return ev
	}

	ev.buildLayout()
	ev.loadInstances() // Load instances asynchronously

	return ev
}

func (ev *EC2View) buildLayout() {
	// Create instance list
	ev.instanceList = tview.NewList()
	ev.instanceList.SetBorder(false).SetTitle("EC2 Instances (Loading...)")

	// Create details panel
	ev.detailsPanel = tview.NewTextView().
		SetText("[yellow]Loading instances...[white]\n\nPlease wait...").
		SetDynamicColors(true).
		SetWordWrap(true)

	ev.detailsPanel.SetBorder(true).SetTitle("Instance Details")

	// Update details when selection changes
	ev.instanceList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(ev.instances) {
			ev.showInstanceDetails(index)
		}
	})

	// Layout
	flex := tview.NewFlex().
		AddItem(ev.instanceList, 0, 1, true).
		AddItem(ev.detailsPanel, 0, 2, false)

	frame := tview.NewFrame(flex).
		SetBorders(2, 2, 2, 2, 6, 2).
		AddText("Flying Nimbus - Multi-Cloud Tool", true, tview.AlignCenter, tview.Styles.PrimaryTextColor).
		AddText("Press 'q' to quit", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	// Add keyboard shortcuts
	flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'b':
			ev.goBack()
			return nil
		case 'r':
			ev.refreshInstances()
			return nil
		case 'i': // IAM policies
			currentIndex := ev.instanceList.GetCurrentItem()
			if currentIndex >= 0 && currentIndex < len(ev.instances) {
				ev.showIAMPolicies(currentIndex)
			}
			return nil
		}
		return event
	})

	ev.Layout = frame
}

func (ev *EC2View) loadInstances() {
	// Load instances in a goroutine to avoid blocking UI
	go func() {
		ctx := context.Background()
		instances, err := ev.ec2Service.ListInstances(ctx)
		if err != nil {
			// Update UI on main thread
			ev.app.QueueUpdateDraw(func() {
				ev.instanceList.Clear()
				ev.instanceList.SetTitle("EC2 Instances (Error)")
				ev.detailsPanel.SetText(fmt.Sprintf("[red]Error loading instances:[white]\n\n%v", err))
			})
			return
		}

		ev.instances = instances

		// Update UI on main thread
		ev.app.QueueUpdateDraw(func() {
			ev.populateInstanceList()
		})
	}()
}

func (ev *EC2View) populateInstanceList() {
	ev.instanceList.Clear()

	if len(ev.instances) == 0 {
		ev.instanceList.AddItem("No instances found", "", 0, nil)
		ev.instanceList.SetTitle("EC2 Instances (0)")
		ev.detailsPanel.SetText("[yellow]No EC2 instances found[white]\n\nYou don't have any EC2 instances in this region.")
		return
	}

	// Add each instance to the list
	for i, instance := range ev.instances {
		// Create description with state and type
		description := fmt.Sprintf("%s | %s", instance.State, instance.InstanceType)

		// Capture index for closure
		index := i

		ev.instanceList.AddItem(
			instance.Name,
			description,
			rune('1'+i), // Shortcuts 1-9
			func() {
				ev.selectInstance(index)
			},
		)
	}

	// Add back option
	ev.instanceList.AddItem("Back to AWS Menu", "Return to AWS services", 'b', ev.goBack)

	ev.instanceList.SetTitle(fmt.Sprintf("EC2 Instances (%d)", len(ev.instances)))

	// Show details of first instance
	if len(ev.instances) > 0 {
		ev.showInstanceDetails(0)
	}
}

func (ev *EC2View) getDisplayValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (ev *EC2View) formatTags(tags map[string]string) string {
	if len(tags) == 0 {
		return "  No tags"
	}

	var tagLines []string

	// Sort tags by key for consistent display
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Format each tag
	for _, key := range keys {
		value := tags[key]
		// Truncate long values
		if len(value) > 40 {
			value = value[:37] + "..."
		}
		tagLines = append(tagLines, fmt.Sprintf("  %s: %s", key, value))
	}

	return strings.Join(tagLines, "\n")
}

func (ev *EC2View) showInstanceDetails(index int) {
	if index < 0 || index >= len(ev.instances) {
		return
	}

	instance := ev.instances[index]

	// Color based on state
	stateColor := "white"
	switch instance.State {
	case "running":
		stateColor = "green"
	case "stopped":
		stateColor = "red"
	case "pending":
		stateColor = "yellow"
	case "stopping", "shutting-down":
		stateColor = "orange"
	}

	iamProfileText := "-"
	if instance.IamInstanceProfileName != "" {
		iamProfileText = fmt.Sprintf("%s\n                [blue](Press 'i' to view policies)[white]",
			instance.IamInstanceProfileName)
	}

	details := fmt.Sprintf(
		"[yellow]Instance: %s[white]\n\n"+
			"[yellow]Basic Information[white]\n"+
			"Instance ID:  %s\n"+
			"State:        [%s]%s[white]\n"+
			"Type:         %s\n"+
			"Launch Time:  %s\n\n"+
			"[yellow]Network[white]\n"+
			"VPC ID:       %s\n"+
			"Subnet ID:    %s\n"+
			"Private IP:   %s\n"+
			"Public IP:    %s\n\n"+
			"[yellow]IAM[white]\n"+
			"Profile:      %s\n\n"+
			"[yellow]Tags[white]\n"+
			"%s\n"+
			"[yellow]Actions[white]\n"+
			"• Press Enter to select instance\n"+
			"• Press 'r' to refresh list\n"+
			"• Press 'i' to view IAM policies\n"+
			"• Press 'b' to go back",
		instance.Name,
		instance.InstanceID,
		stateColor, instance.State,
		instance.InstanceType,
		ev.getDisplayValue(instance.LaunchTime),
		ev.getDisplayValue(instance.VpcID),
		ev.getDisplayValue(instance.SubnetID),
		ev.getDisplayValue(instance.PrivateIP),
		ev.getDisplayValue(instance.PublicIP),
		iamProfileText,
		ev.formatTags(instance.Tags),
	)

	ev.detailsPanel.SetText(details)
}

func (ev *EC2View) selectInstance(index int) {
	if index < 0 || index >= len(ev.instances) {
		return
	}

	instance := ev.instances[index]

	// Show action modal
	ec2modal := tview.NewModal().
		SetText(fmt.Sprintf("Actions for %s", instance.Name)).
		AddButtons([]string{"Start", "Stop", "SSM Connect", "SSM Tunnel", "Terminate", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			switch buttonLabel {
			case "Start":
				ev.startInstance(instance.InstanceID)
			case "Stop":
				ev.stopInstance(instance.InstanceID)
			case "Terminate":
				ev.terminateInstance(instance.InstanceID)
			case "SSM Connect":
				ev.ssmInstance(instance.InstanceID)
			case "SSM Tunnel":
				ev.showSSMTunnelModal(instance.InstanceID, instance.Name)
			}
			ev.pages.RemovePage("modal")
		})

	ev.pages.AddPage("ec2modal", ec2modal, true, true)
}

func (ev *EC2View) refreshInstances() {
	// Show refreshing state
	ev.instanceList.SetTitle("EC2 Instances (Refreshing...)")
	ev.detailsPanel.Clear()
	ev.detailsPanel.SetText("[yellow]Refreshing instance list...[white]\n\nPlease wait...")

	// Clear the list
	ev.instanceList.Clear()

	// Load instances in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		instances, err := ev.ec2Service.ListInstances(ctx)

		// Update UI on main thread
		ev.app.QueueUpdateDraw(func() {
			if err != nil {
				ev.instanceList.SetTitle("EC2 Instances (Error)")
				ev.detailsPanel.Clear()
				ev.detailsPanel.SetText(fmt.Sprintf(
					"[red]Error loading instances:[white]\n\n%v\n\n"+
						"Press 'r' to try again",
					err))
				return
			}

			// Update instances data
			ev.instances = instances

			// Rebuild the list completely
			ev.populateInstanceList()
		})
	}()
}

func (ev *EC2View) startInstance(instanceID string) {
	go func() {
		// ev.detailsPanel.SetText(fmt.Sprintf("[green]Starting instance %s...[white]", instanceID))
		ctx := context.Background()
		result, err := ev.ec2Service.StartInstances(ctx, instanceID)

		ev.app.QueueUpdateDraw(func() {
			if err != nil {
				ev.detailsPanel.SetText(fmt.Sprintf("[red]Failed to start instances[white]\n\n"+
					"Instance %s\n\n"+
					"Error: %v\n\n"+
					"Please check: \n"+
					"- IAM Permissions(ec2:StartInstances)"+
					"- Instance state allows starting\n"+
					"- AWS Service Status", instanceID, err))
				return
			}

			ev.detailsPanel.SetText(fmt.Sprintf("[green]Instance staring successfully![white]\n\n"+
				"Instance: %s\n\n"+
				"Action: Starting Requested \n\n"+
				"Status: %s\n\n"+
				"[yellow]Note:[white] It may take 30-60 seconds for the instance to fully start\n\n"+
				"Press 'r' to refresh the instance list", instanceID, result.StartingInstances[0].CurrentState.Name))
		})
	}()
}

func (ev *EC2View) stopInstance(instanceID string) {
	ev.detailsPanel.SetText(fmt.Sprintf("[red]Stopping instance %s...[white]", instanceID))
	go func() {
		ctx := context.Background()
		result, err := ev.ec2Service.StopInstance(ctx, instanceID)
		ev.app.QueueUpdateDraw(func() {
			if err != nil {
				ev.detailsPanel.SetText(fmt.Sprintf("[red]Failed to stop instances[white]\n\n"+
					"Instance %s\n\n"+
					"Error: %v\n\n"+
					"Please check: \n"+
					"- IAM Permissions(ec2:StartInstances)"+
					"- Instance state allows starting\n"+
					"- AWS Service Status", instanceID, err))
				return
			}

			ev.detailsPanel.SetText(fmt.Sprintf("[green]Instance stopping successfully![white]\n\n"+
				"Instance: %s\n\n"+
				"Action: Starting Requested \n\n"+
				"Status: %s\n\n"+
				"[yellow]Note:[white] It may take 30-60 seconds for the instance to fully start\n\n"+
				"Press 'r' to refresh the instance list", instanceID, result.StoppingInstances[0].CurrentState.Name))
		})
	}()
}

func (ev *EC2View) terminateInstance(instanceID string) {
	ev.detailsPanel.SetText(fmt.Sprintf("[red]Terminating instance %s...[white]", instanceID))

	go func() {
		ctx := context.Background()
		result, err := ev.ec2Service.TerminateInstance(ctx, instanceID)
		ev.app.QueueUpdateDraw(func() {
			if err != nil {
				ev.detailsPanel.SetText(fmt.Sprintf("[red]Issue with termiating an instance %s\n\n"+
					"Please check IAM permissions. Error %s \n\n", instanceID, result.TerminatingInstances[0].CurrentState.Name))
			}
		})
	}()
}

func (ev *EC2View) ssmInstance(instanceID string) {
	if ev.ec2SSMService == nil {
		ev.detailsPanel.Clear()
		ev.detailsPanel.SetText(fmt.Sprintf("SSM service is not available %s", instanceID))
	} else {
		ev.detailsPanel.SetText("SSM service is available")
	}

	ev.detailsPanel.Clear()
	ev.detailsPanel.SetText(fmt.Sprintf("Connecting to ec2 via SSM ... [white]\n\n"+
		"InstanceID: %s\n\n"+
		"The application will now switch to terminal mode\n"+
		"Press Ctrl+D or type 'exit' to end the session.", instanceID))
	// Wait then Stop application so it can drop you in to terminal
	time.Sleep(1 * time.Second)
	ev.app.Suspend(
		func() {
			ctx := context.Background()
			fmt.Printf("AWS SSM Session - %s\n", instanceID)
			err := ev.ec2SSMService.StartSession(ctx, instanceID)
			if err != nil {
				fmt.Printf("**************************")
				fmt.Printf("[ERROR] SSM Session Failed\n")
				fmt.Printf("Error: %v \n", err)
				fmt.Printf("**************************\n\n")
				fmt.Println("Troubleshooting:")
				fmt.Println("  1. Ensure the EC2 instance has SSM agent installed and running")
				fmt.Println("  2. Verify the instance has an IAM role with 'AmazonSSMManagedInstanceCore' policy")
				fmt.Println("  3. Check that the instance is in a 'running' state")
				fmt.Println("  4. Ensure session-manager-plugin is installed")
				fmt.Println("\nInstall plugin from:")
				fmt.Println("		https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html")
			}

			fmt.Print("Press Enter to return to CloudTUI...")
			fmt.Scanln()
		})
}

func (ev *EC2View) ssmTunnelInstance(instanceID string) {
	ev.detailsPanel.SetText(fmt.Sprintf("[green]Executing SSM Tunnel to %s...[white]", instanceID))
}

func (ev *EC2View) buildErrorLayout(err error) {
	errorText := tview.NewTextView().SetText(fmt.Sprintf("[red]Error initializing AWS EC2 service:[white]\n\n%v\n\n"+"Please check:\n"+"• AWS credentials are configured\n"+"• AWS CLI is installed\n"+
		"• You have proper IAM permissions\n\n"+
		"Press 'b' to go back", err)).
		SetDynamicColors(true)

	errorText.SetBorder(true).SetTitle("Error")

	errorText.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'b' {
			ev.goBack()
		}
		return event
	})

	ev.Layout = errorText
}

func (ev *EC2View) goBack() {
	ev.pages.SwitchToPage("root")
}

func (ev *EC2View) showIAMPolicies(index int) {
	if index < 0 || index >= len(ev.instances) {
		return
	}

	instance := ev.instances[index]

	if instance.IamInstanceProfileName == "" {
		modal := tview.NewModal().
			SetText("No IAM Instance Profile\n\n" +
				"This instance does not have an IAM instance profile attached.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				ev.pages.RemovePage("no-iam-modal")
			})

		ev.pages.AddPage("no-iam-modal", modal, true, true)
		return
	}

	if ev.iamService == nil {
		modal := tview.NewModal().
			SetText("IAM Service Not Available\n\n" +
				"Unable to initialize IAM service.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				ev.pages.RemovePage("iam-error-modal")
			})

		ev.pages.AddPage("iam-error-modal", modal, true, true)
		return
	}

	// Show loading modal
	loadingText := tview.NewTextView().
		SetText(fmt.Sprintf("[yellow]Loading IAM policies for %s...[white]\n\nPlease wait...",
			instance.IamInstanceProfileName)).
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	loadingText.SetBorder(true).SetTitle("Loading")

	loadingModal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(loadingText, 7, 1, false).
			AddItem(nil, 0, 1, false), 60, 1, false).
		AddItem(nil, 0, 1, false)

	ev.pages.AddPage("iam-loading", loadingModal, true, true)

	// Load IAM info in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		profileInfo, err := ev.iamService.GetInstanceProfileInfo(ctx, instance.IamInstanceProfileName)

		ev.app.QueueUpdateDraw(func() {
			ev.pages.RemovePage("iam-loading")

			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to Load IAM Information\n\n%v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						ev.pages.RemovePage("iam-error")
					})

				ev.pages.AddPage("iam-error", errorModal, true, true)
				return
			}

			ev.displayIAMPolicies(instance, profileInfo)
		})
	}()
}

func (ev *EC2View) displayIAMPolicies(instance awsService.EC2Instance, profileInfo *awsService.InstanceProfileInfo) {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("[yellow]IAM Instance Profile: %s[white]\n\n", profileInfo.ProfileName))
	content.WriteString(fmt.Sprintf("Instance: %s\n", instance.Name))
	content.WriteString(fmt.Sprintf("Profile ARN: %s\n\n", profileInfo.ProfileArn))

	if profileInfo.RoleName != "" {
		content.WriteString(fmt.Sprintf("[yellow]IAM Role: %s[white]\n", profileInfo.RoleName))
		content.WriteString(fmt.Sprintf("Role ARN: %s\n\n", profileInfo.RoleArn))
	}

	// Show attached managed policies
	if len(profileInfo.AttachedPolicies) > 0 {
		content.WriteString(fmt.Sprintf("[yellow]Attached Managed Policies (%d)[white]\n", len(profileInfo.AttachedPolicies)))
		for _, policy := range profileInfo.AttachedPolicies {
			content.WriteString(fmt.Sprintf("  [green]•[white] %s\n", policy.PolicyName))
			content.WriteString(fmt.Sprintf("    %s\n", policy.PolicyArn))
		}
		content.WriteString("\n")
	} else {
		content.WriteString("[yellow]Attached Managed Policies[white]\n")
		content.WriteString("  No managed policies attached\n\n")
	}

	// Show inline policies
	if len(profileInfo.InlinePolicies) > 0 {
		content.WriteString(fmt.Sprintf("[yellow]Inline Policies (%d)[white]\n", len(profileInfo.InlinePolicies)))
		for _, policyName := range profileInfo.InlinePolicies {
			content.WriteString(fmt.Sprintf("  [blue]•[white] %s\n", policyName))
		}
		content.WriteString("\n")
	} else {
		content.WriteString("[yellow]Inline Policies[white]\n")
		content.WriteString("  No inline policies\n\n")
	}

	// Create scrollable text view
	textView := tview.NewTextView().
		SetText(content.String()).
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(true)

	textView.SetBorder(true).SetTitle(fmt.Sprintf("IAM Policies - %s", instance.Name))

	// Add input handler
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyEsc {
			ev.pages.RemovePage("iam-policies")
			return nil
		}
		return event
	})

	// Create layout
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(textView, 0, 8, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	footer := tview.NewTextView().
		SetText("[yellow]Press 'q' or ESC to close | Use arrow keys to scroll[white]").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	finalFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(flex, 0, 1, true).
		AddItem(footer, 1, 0, false)

	ev.pages.AddPage("iam-policies", finalFlex, true, true)
}

func (ev *EC2View) showSSMTunnelModal(instanceID, instanceName string) {
	form := tview.NewForm()
	form.
		AddInputField("Local Port", "8080", 10, nil, nil).
		AddInputField("Remote Port", "80", 10, nil, nil).
		AddButton("Start Tunnel", func() {
			// Get the values from the form
			localPortField := form.GetFormItem(0).(*tview.InputField)
			remotePortField := form.GetFormItem(1).(*tview.InputField)

			localPort := localPortField.GetText()
			remotePort := remotePortField.GetText()

			// Validate ports
			if localPort == "" || remotePort == "" {
				ev.showError(fmt.Errorf("both ports must be specified"))
				return
			}

			// Remove the modal
			ev.pages.RemovePage("tunnel-modal")

			// Start the tunnel
			ev.startSSMTunnel(instanceID, instanceName, localPort, remotePort)
		}).
		AddButton("Cancel", func() {
			ev.pages.RemovePage("tunnel-modal")
		})

	form.SetBorder(true).SetTitle(fmt.Sprintf("SSM Port Forwarding - %s", instanceName))
	form.SetFieldBackgroundColor(tcell.ColorBlack)

	// Add some helpful text
	helpText := tview.NewTextView().
		SetText("[yellow]Configure Port Forwarding[white]\n\n" +
			"Local Port: Port on your machine\n" +
			"Remote Port: Port on the instance\n\n" +
			"Example:\n" +
			"  Local: 8080, Remote: 80\n" +
			"  Access via: localhost:8080").
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	// Create a flex layout with help text and form
	content := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(helpText, 9, 0, false).
		AddItem(form, 0, 1, true)

	// Center the modal
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(content, 18, 1, true).
			AddItem(nil, 0, 1, false), 60, 1, true).
		AddItem(nil, 0, 1, false)

	ev.pages.AddPage("tunnel-modal", modal, true, true)
}

func (ev *EC2View) showError(err error) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[red]Error[white]\n\n%v", err)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ev.pages.RemovePage("error-modal")
		})

	ev.pages.AddPage("error-modal", modal, true, true)
}

func (ev *EC2View) showSuccess(message string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("[green]Success[white]\n\n%s", message)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ev.pages.RemovePage("success-modal")
		})

	ev.pages.AddPage("success-modal", modal, true, true)
}

func (ev *EC2View) startSSMTunnel(instanceID, instanceName, localPort, remotePort string) {
	if ev.ec2SSMService == nil {
		ev.detailsPanel.Clear()
		ev.detailsPanel.SetText("[red]SSM service not available[white]")
		return
	}

	// Validate ports are numbers
	var localPortInt, remotePortInt int
	_, err := fmt.Sscanf(localPort, "%d", &localPortInt)
	if err != nil {
		ev.showError(fmt.Errorf("invalid local port: %s", localPort))
		return
	}

	_, err = fmt.Sscanf(remotePort, "%d", &remotePortInt)
	if err != nil {
		ev.showError(fmt.Errorf("invalid remote port: %s", remotePort))
		return
	}

	// Validate port ranges
	if localPortInt < 1 || localPortInt > 65535 {
		ev.showError(fmt.Errorf("local port must be between 1 and 65535"))
		return
	}

	if remotePortInt < 1 || remotePortInt > 65535 {
		ev.showError(fmt.Errorf("remote port must be between 1 and 65535"))
		return
	}

	// Show status message
	ev.detailsPanel.Clear()
	ev.detailsPanel.SetText(fmt.Sprintf(
		"╔════════════════════════════════════════════════════════════════╗"+
			"║  [yellow]Starting SSM port forwarding...[white]║  \n"+
			"║  Instance: %s (%s)║  \n"+
			"║  Local Port: %s║  \n"+
			"║  Remote Port: %s║  \n"+
			"║  Initializing...║  \n"+
			"╚════════════════════════════════════════════════════════════════╝",
		instanceName, instanceID, localPort, remotePort))

	ev.app.Draw()
	time.Sleep(500 * time.Millisecond)

	// Suspend TUI and start port forwarding
	ev.app.Suspend(func() {
		ctx := context.Background()

		// Clear screen and show header
		fmt.Print("\033[H\033[2J")
		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  AWS SSM Port Forwarding")
		fmt.Printf("║  Instance: %s\n", instanceName)
		fmt.Printf("║  localhost:%s -> instance:%s\n", localPort, remotePort)
		fmt.Println("╚════════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("Port forwarding is active.")
		fmt.Println("Press Ctrl+C to stop and return to CloudUI.")
		fmt.Println()

		// Start the tunnel
		err := ev.ec2SSMService.StartPortForwarding(ctx, instanceID, localPortInt, remotePortInt)

		// Tunnel ended
		fmt.Println()
		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  Port Forwarding Ended")
		fmt.Println("╚════════════════════════════════════════════════════════════════╝")

		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		}

		fmt.Println("\nPress Enter to return to CloudUI...")
		fmt.Scanln()
	})

	// Refresh when returning to TUI
	ev.refreshInstances()
}
