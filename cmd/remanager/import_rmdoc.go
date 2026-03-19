package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"

	"reManager/internal/pdfimport"
	"reManager/internal/rmdocimport"
)

func importRmdocCmd() *cobra.Command {
	var visibleName string
	var restartXochitl bool

	cmd := &cobra.Command{
		Use:   "import-rmdoc [rmdoc-path]",
		Short: "Import an rmdoc notebook to remarkable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rmdocPath := args[0]
			zipData, err := os.ReadFile(rmdocPath)
			if err != nil {
				return fmt.Errorf("failed to read rmdoc: %w", err)
			}

			if visibleName == "" {
				info, err := rmdocimport.Inspect(zipData)
				if err != nil {
					visibleName = strings.TrimSuffix(filepath.Base(rmdocPath), filepath.Ext(rmdocPath))
				} else {
					visibleName = info.VisibleName
				}
			}

			client, deviceType, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			fmt.Printf("Connected to %s (%s)\n", host, deviceType)
			sftpClient, err := sftp.NewClient(client)
			if err != nil {
				return fmt.Errorf("failed to create SFTP client: %w", err)
			}
			defer sftpClient.Close()

			docID, err := rmdocimport.Upload(sftpClient, zipData, visibleName)
			if err != nil {
				return err
			}

			fmt.Println("Upload complete.")
			fmt.Printf("Document ID: %s\n", docID)
			fmt.Printf("Uploaded files under: %s\n", pdfimport.XochitlPath)

			if restartXochitl {
				session, err := client.NewSession()
				if err != nil {
					return fmt.Errorf("failed to create SSH session for restart: %w", err)
				}
				defer session.Close()
				if _, err := session.CombinedOutput("systemctl restart xochitl"); err != nil {
					return fmt.Errorf("uploaded, but failed to restart xochitl: %w", err)
				}
				fmt.Println("xochitl restarted.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&visibleName, "name", "", "Visible document name (defaults to name from rmdoc metadata)")
	cmd.Flags().BoolVar(&restartXochitl, "restart-xochitl", true, "Restart xochitl after upload")

	return cmd
}
