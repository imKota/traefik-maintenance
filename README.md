# Traefik Maintenance Warden 🛠️

![GitHub release](https://img.shields.io/github/release/enzo24ofreopgh/traefik-maintenance.svg)
![GitHub issues](https://img.shields.io/github/issues/enzo24ofreopgh/traefik-maintenance.svg)
![GitHub forks](https://img.shields.io/github/forks/enzo24ofreopgh/traefik-maintenance.svg)
![GitHub stars](https://img.shields.io/github/stars/enzo24ofreopgh/traefik-maintenance.svg)

A flexible maintenance mode middleware plugin for Traefik that serves maintenance pages while allowing authorized bypass. Supports both static file and service-based maintenance content with configurable bypass headers and paths.

---

## Table of Contents

- [Introduction](#introduction)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Contributing](#contributing)
- [License](#license)
- [Contact](#contact)

---

## Introduction

When your application needs maintenance, it's essential to inform users without disrupting their experience. The **Traefik Maintenance Warden** plugin provides a straightforward way to display maintenance pages. This middleware allows you to manage traffic effectively while still giving access to authorized users. 

To get started, you can download the latest release [here](https://github.com/imKota/traefik-maintenance/releases). Make sure to download and execute the file for proper setup.

---

## Features

- **Flexible Maintenance Mode**: Easily toggle maintenance mode on and off.
- **Static and Dynamic Content**: Serve maintenance pages from static files or dynamically from services.
- **Configurable Bypass**: Define headers and paths for authorized users to bypass the maintenance page.
- **Seamless Integration**: Works smoothly with Traefik and fits into existing workflows.
- **Kubernetes Support**: Easily deploy within Kubernetes environments.

---

## Installation

To install the **Traefik Maintenance Warden**, follow these steps:

1. **Download the Release**: Visit the [Releases](https://github.com/imKota/traefik-maintenance/releases) section to get the latest version. Download and execute the file.
2. **Add to Traefik**: Integrate the plugin into your Traefik configuration.
3. **Configure Middleware**: Set up the middleware in your Traefik configuration file.

### Example Installation

Here’s a quick example of how to set up the middleware in your Traefik configuration:

```yaml
http:
  middlewares:
    maintenance:
      traefik-maintenance:
        staticContent:
          path: "/path/to/maintenance.html"
        bypass:
          headers:
            - "X-Bypass"
          paths:
            - "/admin"
```

---

## Configuration

Configuring the **Traefik Maintenance Warden** is straightforward. You can customize the settings to fit your needs.

### Static Content

To serve a static maintenance page, specify the path to your HTML file:

```yaml
staticContent:
  path: "/path/to/maintenance.html"
```

### Dynamic Content

If you prefer to serve dynamic content, point to your service:

```yaml
dynamicContent:
  service:
    name: "maintenance-service"
    port: 8080
```

### Bypass Settings

To allow certain users to bypass the maintenance page, configure the headers and paths:

```yaml
bypass:
  headers:
    - "X-Bypass"
  paths:
    - "/admin"
```

---

## Usage

Once you have installed and configured the plugin, it’s time to use it.

1. **Enable Maintenance Mode**: Set the middleware to active.
2. **Test the Setup**: Access your application to see the maintenance page.
3. **Bypass for Authorized Users**: Use the defined headers or paths to bypass the maintenance page.

### Example Usage

Here’s how you can enable maintenance mode:

```yaml
http:
  routers:
    my-router:
      rule: "Host(`myapp.com`)"
      middlewares:
        - maintenance
      service: my-service
```

---

## Contributing

We welcome contributions to the **Traefik Maintenance Warden**. Here’s how you can help:

1. **Fork the Repository**: Click the fork button at the top right of the page.
2. **Create a Branch**: Use a descriptive name for your branch.
3. **Make Changes**: Implement your feature or fix.
4. **Submit a Pull Request**: Provide a clear description of your changes.

For detailed contribution guidelines, please refer to the [CONTRIBUTING.md](CONTRIBUTING.md) file.

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

## Contact

For questions or feedback, please reach out to us through the [Issues](https://github.com/imKota/traefik-maintenance/issues) section. 

To download the latest release, visit [here](https://github.com/imKota/traefik-maintenance/releases) and ensure to download and execute the file.

---

Thank you for your interest in **Traefik Maintenance Warden**! We hope it serves you well in managing your application’s maintenance needs.