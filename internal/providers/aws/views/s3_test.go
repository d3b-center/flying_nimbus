package views

import (
	aws "flying_nimbus/internal/providers/aws/backend"
	"testing"
)

func TestGetSubtreeListItems(t *testing.T) {
	fileTree := &aws.S3FileTree{
		Files: []string{"file1", "file2"},
		Subdirs: map[string]*aws.S3FileTree{
			"dir1": &aws.S3FileTree{
				Files: []string{},
				Subdirs: map[string]*aws.S3FileTree{
					"dir2": &aws.S3FileTree{
						Files: []string{"file3", "file4"},
						Subdirs: map[string]*aws.S3FileTree{
							"dir3": &aws.S3FileTree{
								Files:   []string{"file5", "file6"},
								Subdirs: map[string]*aws.S3FileTree{},
							},
							"dir4": &aws.S3FileTree{
								Files:   []string{"file7", "file8"},
								Subdirs: map[string]*aws.S3FileTree{},
							},
						},
					},
				},
			},
		},
	}

	path := []string{"dir1", "dir2"}

	listItems := getSubtreeListItems(path, fileTree)

	if len(listItems) != 4 {
		t.Errorf("wrong list length, should be 4 got %d", len(listItems))
	}

	item0, ok := listItems[0].(regularFileListItem)
	if !ok {
		t.Errorf("list item should be regularFileListItem, is not")
	} else if item0.name != "file3" {
		t.Errorf("name should be file3, got %q", item0.name)
	}

	item1, ok := listItems[1].(regularFileListItem)
	if !ok {
		t.Errorf("list item should be regularFileListItem, is not")
	} else if item1.name != "file4" {
		t.Errorf("name should be file4, got %q", item0.name)
	}
}
