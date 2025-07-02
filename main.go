package main

import (
	"context"
	"github.com/akozlenkov/faptly/config"
	"github.com/akozlenkov/faptly/manager"
	"github.com/urfave/cli/v3"
	"log"
	"os"
)

func main() {
	cmd := &cli.Command{
		Name:  "faptly",
		Usage: "Fast APT repository management tool",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Usage:   "Load config from `FILE`",
				Sources: cli.EnvVars("FAPTLY_CONFIG"),
			},
			&cli.StringFlag{
				Name:    "s3_endpoint",
				Usage:   "S3 endpoint URL",
				Sources: cli.EnvVars("FAPTLY_S3_ENDPOINT"),
			},
			&cli.StringFlag{
				Name:    "s3_bucket",
				Usage:   "S3 bucket name",
				Sources: cli.EnvVars("FAPTLY_S3_BUCKET"),
			},
			&cli.StringFlag{
				Name:    "s3_access_key",
				Usage:   "S3 access key",
				Sources: cli.EnvVars("FAPTLY_S3_ACCESS_KEY"),
			},
			&cli.StringFlag{
				Name:    "s3_secret_key",
				Usage:   "S3 secret key",
				Sources: cli.EnvVars("FAPTLY_S3_SECRET_KEY"),
			},
			&cli.StringFlag{
				Name:    "private_gpg_key",
				Usage:   "Load GPG key from `FILE`",
				Sources: cli.EnvVars("FAPTLY_PRIVATE_GPG_KEY"),
			},
			&cli.StringFlag{
				Name:    "private_gpg_passkey",
				Usage:   "Private GPG passkey",
				Sources: cli.EnvVars("FAPTLY_PRIVATE_GPG_PASSKEY"),
			},
		},
		Before: func(ctx context.Context, command *cli.Command) (context.Context, error) {
			cfg := config.New()

			if command.String("config") != "" {
				if err := cfg.Load(command.String("config")); err != nil {
					return ctx, err
				}
			}

			for _, k := range []string{
				"s3_endpoint",
				"s3_bucket",
				"s3_access_key",
				"s3_secret_key",
				"private_gpg_passkey",
			} {
				if command.String(k) != "" {
					switch k {
					case "s3_endpoint":
						cfg.S3Endpoint = command.String(k)
					case "s3_bucket":
						cfg.S3Bucket = command.String(k)
					case "s3_access_key":
						cfg.S3AccessKey = command.String(k)
					case "s3_secret_key":
						cfg.S3SecretKey = command.String(k)
					case "private_gpg_passkey":
						cfg.PrivateGPGPasskey = command.String(k)
					}
				}
			}

			if command.String("private_gpg_key") != "" {
				f, err := os.ReadFile(command.String("private_gpg_key"))
				if err != nil {
					return ctx, err
				}
				cfg.PrivateGPGKey = string(f)
			}

			return context.WithValue(ctx, "config", cfg), cfg.Validate()
		},
		Commands: []*cli.Command{
			{
				Name: "pkg",
				Commands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List all available packages",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "suite",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "component",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "architecture",
								Required: true,
							},
						},
						Action: func(ctx context.Context, command *cli.Command) error {
							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.ListPkgs(
								command.String("suite"),
								command.String("component"),
								command.String("architecture"),
							)
						},
					},
					{
						Name:      "show",
						Usage:     "Show package configuration",
						ArgsUsage: "<package>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "suite",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "component",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "architecture",
								Required: true,
							},
						},
						Action: func(ctx context.Context, command *cli.Command) error {
							if command.Args().Len() == 0 {
								return cli.ShowSubcommandHelp(command)
							}

							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.ShowPkg(
								command.String("suite"),
								command.String("component"),
								command.String("architecture"),
								command.Args().First(),
							)
						},
					},
					{
						Name:  "upload",
						Usage: "Upload package to repository",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "suite",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "component",
								Required: true,
							},
						},
						Action: func(ctx context.Context, command *cli.Command) error {
							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.UploadPkgs(
								command.String("suite"),
								command.String("component"),
								command.Args().Slice(),
							)
						},
					},
				},
			},
			{
				Name: "repo",
				Commands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List all available repositories",
						Action: func(ctx context.Context, command *cli.Command) error {
							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.ListRepos()
						},
					},
					{
						Name:      "show",
						Usage:     "Show repository configuration",
						ArgsUsage: "<suite>",
						Action: func(ctx context.Context, command *cli.Command) error {
							if command.Args().Len() == 0 {
								return cli.ShowSubcommandHelp(command)
							}

							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.ShowRepo(command.Args().First())
						},
					},
					{
						Name:  "create",
						Usage: "Create a repository",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "origin",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "suite",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "label",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "codename",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "description",
								Required: true,
							},
							&cli.StringSliceFlag{
								Name:     "component",
								Required: true,
							},
							&cli.StringSliceFlag{
								Name:     "architecture",
								Required: true,
							},
						},
						Action: func(ctx context.Context, command *cli.Command) error {
							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.CreateRepo(
								command.String("origin"),
								command.String("suite"),
								command.String("label"),
								command.String("codename"),
								command.String("description"),
								command.StringSlice("component"),
								command.StringSlice("architecture"),
							)
						},
					},
					{
						Name:      "delete",
						Usage:     "Delete repository",
						ArgsUsage: "<suite>",
						Action: func(ctx context.Context, command *cli.Command) error {
							if command.Args().Len() == 0 {
								return cli.ShowSubcommandHelp(command)
							}

							mgr, err := manager.New(ctx.Value("config").(*config.Config))
							if err != nil {
								return err
							}
							return mgr.DeleteRepo(command.Args().First())
						},
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
