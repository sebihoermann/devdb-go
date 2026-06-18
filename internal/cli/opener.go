package cli

import (
	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/spf13/cobra"
)

type opener func(cmd *cobra.Command) (*app.Context, error)
