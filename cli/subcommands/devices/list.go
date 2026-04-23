// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package devices

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/foundriesio/dg-satellite/cli/api"
	"github.com/foundriesio/dg-satellite/cli/subcommands"
	"github.com/spf13/cobra"
)

var allColumns = []string{
	"uuid",
	"name",
	"group",
	"target",
	"last-seen",
	"created-at",
	"is-prod",
	"tag",
	"labels",
}

const defaultPageLimit = 50 // the number of devices to fetch per page when listing.

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List devices",
	Long:  `List devices known to the server. By default shows the first page of results.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		columns, err := validateColumns(cmd.Flag("columns").Value.String())
		if err != nil {
			return err
		}
		sortBy, _ := cmd.Flags().GetString("sort")
		if err := validateSortBy(sortBy); err != nil {
			return err
		}
		labelFilters, _ := cmd.Flags().GetStringSlice("label")
		page, _ := cmd.Flags().GetInt("page")
		api := api.CtxGetApi(cmd.Context())
		listDevices(api.Devices(), columns, page, sortBy, labelFilters)
		return nil
	},
}

var validSortValues = []string{
	"name-asc", "name-desc",
	"created-at-asc", "created-at-desc",
	"last-seen-asc", "last-seen-desc",
	"uuid-asc", "uuid-desc",
}

func init() {
	colmnsStr := strings.Join(allColumns, ",")
	sortStr := strings.Join(validSortValues, ", ")
	DevicesCmd.AddCommand(listCmd)
	listCmd.Flags().StringP("columns", "", "uuid,target,last-seen",
		"Comma-separated list of columns to display (available: "+colmnsStr+")")
	listCmd.Flags().IntP("page", "p", 1, "Page number to display")
	listCmd.Flags().StringP("sort", "s", "", "Sort order for devices ("+sortStr+")")
	listCmd.Flags().StringSliceP("label", "l", nil,
		"Filter by label in the format key.comparison.value (e.g. env.eq.production).\n"+
			"Comparisons: eq, ne, contains, ncontains. Can be repeated for AND logic.")
}

func validateSortBy(sortBy string) error {
	if sortBy == "" {
		return nil
	}
	if !slices.Contains(validSortValues, sortBy) {
		return fmt.Errorf("invalid sort value: %s (valid: %s)", sortBy, strings.Join(validSortValues, ", "))
	}
	return nil
}

func validateColumns(columnsStr string) ([]string, error) {
	columns := strings.Split(columnsStr, ",")
	for _, col := range columns {
		if !slices.Contains(allColumns, col) {
			return nil, fmt.Errorf("invalid column: %s", col)
		}
	}
	return columns, nil
}

func listDevices(dapi api.DeviceApi, columns []string, page int, sortBy string, labelFilters []string) {
	devices, hasMore, totalPages, err := dapi.ListPage(page, defaultPageLimit, sortBy, labelFilters)
	cobra.CheckErr(err)

	headers := make([]string, 0, len(columns))
	for _, col := range columns {
		headers = append(headers, strings.ToUpper(strings.ReplaceAll(col, "-", " ")))
	}
	table := subcommands.NewTableWriter(headers)

	for _, device := range devices {
		row := make([]any, 0, len(columns))
		for _, col := range columns {
			row = append(row, getColumnValue(&device, col))
		}
		table.AddRow(row...)
	}

	table.Render()
	if hasMore {
		fmt.Printf("\nA total of %d pages of devices available. Use '--page %d' for the next page.\n", totalPages, page+1)
	}
}

func getColumnValue(device *api.DeviceListItem, column string) string {
	switch column {
	case "uuid":
		return device.Uuid
	case "target":
		return device.Target
	case "last-seen":
		if device.LastSeen > 0 {
			return time.Unix(device.LastSeen, 0).Format("2006-01-02 15:04:05")
		}
		return "-"
	case "created-at":
		if device.CreatedAt > 0 {
			return time.Unix(device.CreatedAt, 0).Format("2006-01-02 15:04:05")
		}
		return "-"
	case "group":
		if group, ok := device.Labels["group"]; ok {
			return group
		}
		return "-"
	case "name":
		if name, ok := device.Labels["name"]; ok {
			return name
		}
		return "-"
	case "is-prod":
		if device.IsProd {
			return "true"
		}
		return "false"
	case "tag":
		return device.Tag
	case "labels":
		if len(device.Labels) == 0 {
			return ""
		}
		labelStrs := ""
		for k, v := range device.Labels {
			if len(labelStrs) > 0 {
				labelStrs += "\n"
			}
			labelStrs += fmt.Sprintf("%s=%s", k, v)
		}
		return labelStrs
	default:
		return ""
	}
}
