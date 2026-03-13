# Flying Nimbus - TUI for Easy AWS/[Azure*] Exploration

[![Build Status](placeholder_build_status.png)](placeholder_build_url)
[![Go Version](placeholder_go_version.png)](placeholder_go_version.png)

**A command-line interface (TUI) tool built in Go to simplify navigation and exploration within your AWS environment.**  This tool aims to provide a more intuitive and focused way to discover resources and services, similar to the `e1s` tool for ECS, but generalized for broader AWS usage.

### Quickstart
1. Download the latest [release](https://github.com/d3b-center/flying_nimbus/releases). Set `FN_VERSION` to the release version.
2. Fix the permissions on your download and create a symlink for convenience.
```
FN_VERSION=v1.0.0
chmod +x ./flying-nimbus-mac-arm64-${FN_VERSION}
xattr -d com.apple.quarantine ./flying-nimbus-mac-arm64-${FN_VERSION}
ln -s ./flying-nimbus-mac-arm64-${FN_VERSION} ./flying-nimbus
```
3. Log in to AWS on your terminal. Make sure AWS_REGION is set as well as AWS credentials environment variables.
4. Ride the Flying Nimbus to your heart's content!
```
./flying-nimbus
```

## 1. Overview

AWS Navigator is designed to be a quick and efficient way to:

*   **Browse AWS Services:**  Quickly see a list of available AWS services.
**  EC2, Route53, RDS, Service Catalog, ALB, Amazon Q only supported
*   **Explore Service Details:**  Get concise information about a selected service, including key services and common configurations.
*   **Navigate Resource Types:**  Explore the different resource types available within a specific service.
*   **(Future Feature) Filter Resources:**  Filter resources based on tags, status, or other attributes (currently under development).

## 2. Installation

**Prerequisites:**

*   Go (1.18 or higher) is required. You can download it from [https://go.dev/dl/](https://go.dev/dl/).
*   A valid AWS account and configured credentials (IAM user with appropriate permissions - see security considerations below).

**Installation Steps:**

1.  **Clone the Repository:**
    ```bash
    git clone git@github.com:d3b-center/flying_nimbus.git
    cd flying_nimbus 
    ```

2.  **Build the Tool:**
    ```bash
    make build
    ```

3.  **Make Executable:**
    ```bash
    chmod +x flying_nimbus 
    ```

## 3. Usage

Once built, the `flying_nimbus` executable can be run from your terminal.

**Basic Usage:**

```bash
flying_nimbus
```

## 4. Configuration

Currently, the tool does not require any configuration files. It uses current AWS auth profile.

## 5. Security Considerations

*   **IAM Role Assumption:**  The recommended approach is to use an IAM role with minimal necessary permissions.  This allows the tool to access AWS resources without requiring your personal credentials to be stored.
*   **Secure Storage of Configuration (Future):**  If you implement configuration files, ensure they are stored securely and have appropriate access controls.

## 6. Roadmap & Future Features

*   **Resource Filtering:**  Implement filtering by tags, status, and other attributes.
*   **Detailed Resource Information:**  Display more comprehensive details for each resource.
*   **Interactive Resource Management (Conceptual):**  Potentially allow basic actions (e.g., starting/stopping services) through the TUI – requires careful security considerations.
*   **Integration with AWS CLI:**  Consider integrating with the AWS CLI for more advanced operations.
*   **Support for Different AWS Accounts:**  Allow selecting and navigating across multiple AWS accounts.
*   **Improved User Interface:**  Enhance the TUI experience with a more visually appealing and user-friendly interface.
*   **JSON/YAML output:**  Allow exporting the list of services or resources to a file for further processing.

## 7. Contributing

We welcome contributions!  If you'd like to help improve the tool:

*   Fork the repository.
*   Submit a pull request with your changes.
*   Follow our [Contribution Guidelines](placeholder_contribution_guidelines.md) (Create this file!)

## 8. License

[MIT License](placeholder_license_file) (Create this file!)
