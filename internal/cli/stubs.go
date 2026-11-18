package cli

import "github.com/spf13/cobra"

func buildCmd() *cobra.Command  { return &cobra.Command{Use: "build"} }
func serveCmd() *cobra.Command  { return &cobra.Command{Use: "serve"} }
func initCmd() *cobra.Command   { return &cobra.Command{Use: "init"} }
func newCmd() *cobra.Command    { return &cobra.Command{Use: "new"} }
func importCmd() *cobra.Command { return &cobra.Command{Use: "import"} }
func schemaCmd() *cobra.Command { return &cobra.Command{Use: "schema"} }
