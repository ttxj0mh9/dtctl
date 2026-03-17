package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/resources/workflow"
)

// getWfeTaskResultCmd retrieves the structured return value of a workflow execution task
var getWfeTaskResultCmd = &cobra.Command{
	Use:     "wfe-task-result <execution-id>",
	Aliases: []string{"workflow-execution-task-result"},
	Short:   "Get the return value of a workflow execution task",
	Long: `Get the structured return value produced by a task in a workflow execution.

Unlike 'dtctl logs wfe', which prints stdout/stderr log output, this command
retrieves the structured data returned by the task (e.g. the object returned
by a JavaScript task's default export function).

Examples:
  # Get the return value of a specific task
  dtctl get wfe-task-result <execution-id> --task <task-name>
  dtctl get workflow-execution-task-result <execution-id> --task <task-name>

  # Output as JSON
  dtctl get wfe-task-result <execution-id> --task <task-name> -o json

  # Output as YAML
  dtctl get wfe-task-result <execution-id> --task <task-name> -o yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		executionID := args[0]

		taskName, _ := cmd.Flags().GetString("task")

		cfg, err := LoadConfig()
		if err != nil {
			return err
		}

		c, err := NewClientFromConfig(cfg)
		if err != nil {
			return err
		}

		handler := workflow.NewExecutionHandler(c)
		result, err := handler.GetTaskResult(executionID, taskName)
		if err != nil {
			return err
		}

		printer := NewPrinter()
		return printer.Print(result)
	},
}

func init() {
	getWfeTaskResultCmd.Flags().StringP("task", "t", "", "Task name to retrieve the result for (required)")
	if err := getWfeTaskResultCmd.MarkFlagRequired("task"); err != nil {
		panic(err)
	}
}
