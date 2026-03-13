package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"

	"reManager/internal/pdfimport"
)

func importPDFCmd() *cobra.Command {
	var visibleName string
	var parentID string
	var pageCountOverride int
	var restartXochitl bool

	cmd := &cobra.Command{
		Use:   "import-pdf [pdf-path]",
		Short: "Import a PDF to remarkable, generating the proper metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pdfPath := args[0]
			pdfData, err := os.ReadFile(pdfPath)
			if err != nil {
				return fmt.Errorf("failed to read PDF: %w", err)
			}

			pageCount := pageCountOverride
			if pageCount <= 0 {
				pageCount = pdfimport.EstimatePageCount(pdfData)
			}
			if pageCount <= 0 {
				return fmt.Errorf("could not determine page count from PDF; retry with --pages")
			}

			if visibleName == "" {
				visibleName = strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
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

			docID, err := pdfimport.Upload(sftpClient, pdfData, visibleName, parentID, pageCount)
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
	cmd.Flags().StringVar(&parentID, "parent", "", "Parent collection (default: root)")
	cmd.Flags().IntVar(&pageCountOverride, "pages", 0, "Override page count if auto-detection fails")
	cmd.Flags().BoolVar(&restartXochitl, "restart-xochitl", true, "Restart xochitl after upload")

	return cmd
}
