package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"

	"reManager/internal/epubimport"
	"reManager/internal/pdfimport"
)

func importEpubCmd() *cobra.Command {
	var visibleName string
	var restartXochitl bool

	cmd := &cobra.Command{
		Use:   "import-epub [epub-path]",
		Short: "Import an ePub to remarkable, generating the proper metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			epubPath := args[0]
			epubData, err := os.ReadFile(epubPath)
			if err != nil {
				return fmt.Errorf("failed to read ePub: %w", err)
			}

			if visibleName == "" {
				visibleName = strings.TrimSuffix(filepath.Base(epubPath), filepath.Ext(epubPath))
			}

			client, deviceType, _, err := connect()
			if err != nil {
				return err
			}
			defer client.Close()

			fmt.Printf("Connected to %s (%s)\n", host, deviceType.DisplayName())
			sftpClient, err := sftp.NewClient(client)
			if err != nil {
				return fmt.Errorf("failed to create SFTP client: %w", err)
			}
			defer sftpClient.Close()

			docID, err := epubimport.Upload(sftpClient, epubData, visibleName, "")
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

	cmd.Flags().StringVar(&visibleName, "name", "", "Visible document name (defaults to filename)")
	cmd.Flags().BoolVar(&restartXochitl, "restart-xochitl", true, "Restart xochitl after upload")

	return cmd
}
