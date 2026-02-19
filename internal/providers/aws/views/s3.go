package views

import (
	"context"
	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const s3BucketListWidthRatio = 0.25

type S3BucketsViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	isLoading            bool
	windowSize           common.ContentWindowSizeMsg
	inputRoutingStrategy common.InputRoutingStrategy
	// cargo cult from ec2 model, maybe use this for the objects in the bucket?
	detailViewport   viewport.Model
	bucketsListWidth int
	detailsWidth     int
	contentHeight    int
}

type s3BucketsLoadedMsg []list.Item

func InitS3ViewModel(appService *app.App, windowSize common.ContentWindowSizeMsg) S3BucketsViewModel {
	slog.Debug("Initialize S3 buckets view model")
	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), windowSize.Height, 0)
	l.Title = "Buckets"
	l.SetShowTitle(true)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)

	loader := spinner.New()
	loader.Style = common.SpinnerStyle
	loader.Spinner = spinner.Points

	vp := viewport.New(windowSize.Width, windowSize.Height)

	m := S3BucketsViewModel{
		app:            appService,
		list:           l,
		loader:         loader,
		isLoading:      true,
		windowSize:     windowSize,
		detailViewport: vp,
	}
	m.updateLayout(windowSize)

	return m
}

func (m S3BucketsViewModel) Init() tea.Cmd {
	slog.Debug("Initialize S3 BubbleTea model")
	return tea.Batch(m.loader.Tick, listS3BucketsCmd(m.app.Context, m.app.AWS.S3))
}

func listS3BucketsCmd(ctx context.Context, s3Service *aws.S3Service) tea.Cmd {
	return func() tea.Msg {
		buckets, _ := s3Service.ListBuckets(ctx)
		return s3BucketsLoadedMsg(bucketsToItems(buckets))
	}
}

func bucketsToItems(buckets []aws.S3Bucket) []list.Item {
	items := make([]list.Item, len(buckets))
	for i, bkt := range buckets {
		items[i] = list.Item(bkt)
	}
	return items
}

// updateLayout recalculates and applies layout dimensions.
// copied from EC2 code, but it looks like it should be common to more than just ec2
func (m *S3BucketsViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - BorderWidth
	usableHeight := msg.Height - BorderHeight

	m.bucketsListWidth = int(float64(usableWidth) * s3BucketListWidthRatio)
	m.detailsWidth = usableWidth - m.bucketsListWidth

	m.contentHeight = usableHeight

	m.detailViewport.Width = m.detailsWidth
	m.detailViewport.Height = msg.Height

	m.list.SetSize(m.bucketsListWidth, usableHeight)
}

func (m S3BucketsViewModel) Commands() common.Commands {
	return []key.Binding{toggleFocus}
}

func (m S3BucketsViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m S3BucketsViewModel) Title() string {
	return "S3 Buckets"
}

func (m S3BucketsViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ec2 uses this to update the "details" pane of the currently selected one,
	// maybe use this for bucket objs
	// oldIndex := m.list.Index()
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))
		m.updateLayout(msg)

	case s3BucketsLoadedMsg:
		m.isLoading = false
		m.list.SetItems(msg)
		slog.Debug(fmt.Sprintf("Size of list %d", len(m.list.Items())))

		if len(msg) > 0 {
			// we don't have ec2 instance details, but maybe this is where we also load
			// the bucket's objects?
			// m.updateInstanceDetails()
			m.updateLayout(m.windowSize)
		}

	case spinner.TickMsg:
		newLoader, cmd := m.loader.Update(msg)
		m.loader = newLoader
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		if key.Matches(msg, forceRefresh) {
			m.isLoading = true
			cmd := listS3BucketsCmd(m.app.Context, m.app.AWS.S3)
			cmds = append(cmds, cmd)
		}
		// looks like this also handles stuff with instance details
		// cmd := m.handleKeypress(msg)
		// cmds = append(cmds, cmd)

	default:
		newList, cmd := m.list.Update(msg)
		m.list = newList
		cmds = append(cmds, cmd)

	}

	return m, tea.Batch(cmds...)
}

func (m S3BucketsViewModel) View() string {
	if m.isLoading {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	listStyle := common.InstancesListStyle.
		MaxHeight(m.windowSize.Height).
		BorderForeground(focusedColor)

	return listStyle.Render(m.list.View())
}
