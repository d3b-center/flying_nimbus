package views

import (
	"context"
	"flying_nimbus/internal/app"
	aws "flying_nimbus/internal/providers/aws/backend"
	"flying_nimbus/internal/tui/common"
	"flying_nimbus/internal/tui/constants"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type S3BucketsViewModel struct {
	app                  *app.App
	list                 list.Model
	loader               spinner.Model
	isLoading            bool
	windowSize           common.ContentWindowSizeMsg
	inputRoutingStrategy common.InputRoutingStrategy
	bucketsListWidth     int
	contentHeight        int
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

	m := S3BucketsViewModel{
		app:        appService,
		list:       l,
		loader:     loader,
		isLoading:  true,
		windowSize: windowSize,
	}
	m.updateLayout(windowSize)

	return m
}

func (m S3BucketsViewModel) Init() tea.Cmd {
	slog.Debug("Initialize S3 buckets BubbleTea model")
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

	m.bucketsListWidth = usableWidth

	m.contentHeight = usableHeight

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
		} else if key.Matches(msg, constants.Keymap.Enter) {
			currentItem := m.list.SelectedItem()
			bucket := currentItem.(aws.S3Bucket)
			cmds = append(cmds, func() tea.Msg {
				subdirModel := InitS3FilesViewModel(m.app, bucket.Name, []string{}, m.windowSize)
				return common.NavigateMsg{subdirModel}
			})
		} else {
			newList, cmd := m.list.Update(msg)
			m.list = newList
			cmds = append(cmds, cmd)
		}

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

type S3FilesViewModel struct {
	app *app.App
	// the current level of directory will have a list of children, both files and subdirs
	list      list.Model
	loader    spinner.Model
	isLoading bool

	listWidth            int
	windowSize           common.ContentWindowSizeMsg
	inputRoutingStrategy common.InputRoutingStrategy
	contentHeight        int
	bucketName           string
	currentPath          []string
	fileTree             *aws.S3FileTree
}

func InitS3FilesViewModel(appService *app.App, bucketName string, path []string, windowSize common.ContentWindowSizeMsg) S3FilesViewModel {
	slog.Debug("initialize S3 files view model")

	items := []list.Item{}

	l := list.New(items, list.NewDefaultDelegate(), windowSize.Height, 0)
	l.Title = fmt.Sprintf("Browsing %s/%s", bucketName, strings.Join(path, "/"))

	loader := spinner.New()
	loader.Style = common.SpinnerStyle
	loader.Spinner = spinner.Points

	m := S3FilesViewModel{
		app:         appService,
		loader:      loader,
		list:        l,
		isLoading:   true,
		windowSize:  windowSize,
		bucketName:  bucketName,
		currentPath: path,
	}

	return m
}

func (m S3FilesViewModel) Init() tea.Cmd {
	slog.Debug("Initialize S3 files BubbleTea model")
	return tea.Batch(m.loader.Tick, listS3FilesCmd(m.app.Context, m.app.AWS.S3, m.bucketName))
}

type s3FilesLoadedMsg struct {
	bucketName string
	fileTree   *aws.S3FileTree
}

func listS3FilesCmd(ctx context.Context, s3Service *aws.S3Service, bucketName string) tea.Cmd {
	return func() tea.Msg {
		fileTree, _ := s3Service.ListBucketObjects(ctx, bucketName)
		return s3FilesLoadedMsg{bucketName, fileTree}
	}
}

// updateLayout recalculates and applies layout dimensions.
// copy pasted again from other model, could probably be deduplicated
func (m *S3FilesViewModel) updateLayout(msg common.ContentWindowSizeMsg) {
	m.windowSize = msg

	usableWidth := msg.Width - BorderWidth
	usableHeight := msg.Height - BorderHeight

	m.listWidth = usableWidth

	m.contentHeight = usableHeight

	m.list.SetSize(m.listWidth, usableHeight)
}

func (m S3FilesViewModel) Commands() common.Commands {
	return []key.Binding{toggleFocus}
}

func (m S3FilesViewModel) InputRoutingStrategy() common.InputRoutingStrategy {
	return m.inputRoutingStrategy
}

func (m S3FilesViewModel) Title() string {
	return fmt.Sprintf("Bucket '%s'", m.bucketName)
}

type regularFileListItem struct {
	name string
}

func (i regularFileListItem) Title() string {
	return i.name
}

func (i regularFileListItem) Description() string {
	return "(regular file)"
}

func (i regularFileListItem) FilterValue() string {
	return i.name
}

type subdirListItem struct {
	name     string
	fileTree *aws.S3FileTree
}

func (i subdirListItem) Title() string {
	return i.name
}

func (i subdirListItem) Description() string {
	return "(directory)"
}

func (i subdirListItem) FilterValue() string {
	return i.name
}

func (m S3FilesViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case common.ContentWindowSizeMsg:
		slog.Debug(fmt.Sprintf("Received WindowSizeMsg %v", msg))
		m.updateLayout(msg)

	case s3FilesLoadedMsg:
		m.isLoading = false
		listItems := []list.Item{}
		fileTree := msg.fileTree

		for _, pathComponent := range m.currentPath {
			subdir, ok := fileTree.Subdirs[pathComponent]
			if !ok {
				break
			}
			fileTree = subdir
		}

		if fileTree != nil {
			for _, filename := range fileTree.Files {
				// the tree having empty string is because the s3 bucket shows an object with
				// a trailing slash. I'm not sure if that is just an entry indicating that the
				// prefix exists, or if there is an actual file with a trailing slash. I think
				// the former, so omit the empty items
				if filename != "" {
					listItems = append(listItems, regularFileListItem{filename})
				}
			}
			for k, v := range fileTree.Subdirs {
				listItems = append(listItems, subdirListItem{k, v})
			}
		}
		m.fileTree = fileTree
		m.list.SetItems(listItems)
		m.updateLayout(m.windowSize)

	case spinner.TickMsg:
		newLoader, cmd := m.loader.Update(msg)
		m.loader = newLoader
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		if key.Matches(msg, forceRefresh) {
			m.isLoading = true
			cmd := listS3BucketsCmd(m.app.Context, m.app.AWS.S3)
			cmds = append(cmds, cmd)
		} else if key.Matches(msg, constants.Keymap.Enter) {
			item := m.list.SelectedItem()
			subdir, ok := item.(subdirListItem)
			if ok {
				cmds = append(cmds, func() tea.Msg {
					// TODO this forces it to load again, this is unnecessary but requires
					// possibly splitting up the model into a new subdir model or otherwise
					// changing the logic
					subdirModel := InitS3FilesViewModel(m.app, m.bucketName, append(m.currentPath, subdir.name), m.windowSize)
					return common.NavigateMsg{subdirModel}
				})
			}
		} else {
			newList, cmd := m.list.Update(msg)
			m.list = newList
			cmds = append(cmds, cmd)
		}

	}

	return m, tea.Batch(cmds...)
}

func (m S3FilesViewModel) View() string {
	if m.isLoading {
		return constants.DocStyle.Render(m.loader.View() + "\n")
	}

	listStyle := common.InstancesListStyle.
		MaxHeight(m.windowSize.Height).
		BorderForeground(focusedColor)

	return listStyle.Render(m.list.View())
}
