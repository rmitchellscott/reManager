# reManager
<img src="assets/icon.svg" alt="reManager Icon" width="125" align="right">
<p align="justify">

[![rm1](https://img.shields.io/badge/rM1-supported-green)](https://remarkable.com/store/remarkable)
[![rm2](https://img.shields.io/badge/rM2-supported-green)](https://remarkable.com/store/remarkable-2)
[![rmpp](https://img.shields.io/badge/rMPP-supported-green)](https://remarkable.com/store/overview/remarkable-paper-pro)
[![rmppm](https://img.shields.io/badge/rMPPM-supported-green)](https://remarkable.com/products/remarkable-paper/pro-move)

A multi-platform desktop app for managing mods on reMarkable tablets.

Powered by the [Vellum](https://github.com/vellum-dev) package manager. See the [package index](https://vellum.delivery) for a full list of supported packages.

> [!CAUTION]
> This is pre-release software. Features may change and bugs are expected.

## Features

- Multi-Device Management: Save multiple sets of SSH credentials with password or key-based authentication.
- Package Management: Browse, install, upgrade, and uninstall packages with automatic dependency resolution.
- Maintenance Tools: Run maintenance commands and system tasks with real-time terminal output.

## Screenshot

  <picture>
    <source
      srcset="assets/screenshot-dark.png"
      media="(prefers-color-scheme: dark)"
    >
    <img
      src="assets/screenshot-light.png"
      alt="reManager UI Screenshot"
    >
  </picture>

## Tech Stack

Built with [Wails](https://wails.io/) (Go + React/TypeScript).

## Building

Requires Go 1.23+ and Node.js.

```bash
wails build
```
