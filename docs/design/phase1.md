
---

# ☁️ Flying Nimbus — Phase 1

![](../image.png)

---
**Flying Nimbus** is a **terminal user interface (TUI)** for managing cloud infrastructure with a strong focus on **developer workflows**.

Phase 1 prioritizes rapid delivery of a small, high-impact set of capabilities to provide immediate value to users.

Pre Conditions

* AWS CLI v2
* Login via SSO
* AWS PROFILE set
* Region set

## Phase 1 Objectives

* Deliver a TUI that supports the following
  * EC2 (management)
  * Dev Tunnel
  * RDS (Display Information)
  * Service Catalog
* Establish clean separation utilizing MVC patterns
* Ensure testable, maintainable, and extensible
* Simple UI (Phase 1\)

## Phase 1 Timeline

* Development and testing complete by mid February
* User documentation by mid February
* Roll out to Platform Eng and Bix team by end of February

### Out of Scope \- Phase 1

The items are deemed `nice to have` ( re-evaluate after Phase 1\)

* Breadcrumb support
* Sidebar (I would like to rethink the UI after Phase 1\)
* Azure Support
* Multiple Dev Tunnel support
* Dynamic AWS configuration (dynamically change AWS Account and Region )
* Aesthetic Focus (Idc if its ugly as long as it works )

## Design

### High Level Flow

Root View \-\> Provider Selection Menu \-\> (only AWS) AWS Provider Menu

### Core Architecture

* TUI Framework (Bubbletea)
* Clear Separation (MVC,SOLID):
  * TUI Models (View)
  * Core Services (Scaffolding)
  * Provider Backend implementations (Backend)
* Log Framework
* Exhaustive Testing Framework
  * Unit Test
  * Integration Test
* Automated builds (easy installs for users)
* No dependency on Cloud SDKS from TUI
* Standardized keyboard navigation
  * Enter (select)
  * Escape (back)
  * q  (Quit)
  * ? (Display Help)

### Root Provider

Provider Selection Interface

* Amazon Web Services (aws)
* Azure (Place holder)

### AWS Provider

AWS configuration attempts to automatically grab the current AWS Profile and Region (AWS\_PROFILE, AWS\_REGION) and validate the credentials at the start of TUI.

Capabilities (Menu)

* EC2 (Management & SSM)
* RDS
* Service Catalog

#### EC2

List available EC2s including the following information

* Instance name
* Instance ID
* Instance Type
* Instance State

Display detailed information for current EC2 instance (to the right of the list of ec2)

* Basic Information
* Network (VPC, Subnets, IPs)
* Tags

Actions

* Refresh list
* View Iam Policies
* Start
* Stop
* SSM Connect

#### RDS

List available RDS instances

Display detailed information

* Hostname of database
* Port
* Security Group Ingress Rules

Actions

* Create Dev Tunnel (SSM Tunnel)

Creation of Dev Tunnel

* Assume Default Port
* Use Bastion Host

#### S3

List S3 buckets and their contents

Navigate list of buckets

 * Name of bucket

Select a bucket, view its contents like a directory tree.
S3 has "directory buckets", but our buckets are all of the ordinary "flat" variety.
They may mimick a file tree by including slashes (`/`) in the object keys.

 * Files (objects without `"/"`)
 * Subdirectories (paths up to first `"/"`), select one to descend into all objects sharing that prefix

#### Service Catalog

List available products

* Name
* Version
* Status

List already provisioned products
Show Provisioning Progress

## Developer Experience

### Branch Strategy

main branch

* Serves as active development branch
* Not guaranteed to be production-ready at all times
* All new features shall be merged into `main`

Feature branches

* All new development shall occur on short-lived branches
* Must be merged back into `main` via PRs
* PRs can be draft (do not pull it out of draft unless ready to be review’d)

### Release

* Tagging a commit on `main` with a semantic version
* Generating a corresponding release artifact
* Attaching built binaries to the release
* Publishing release notes summarizing changes

### Quality Expectations

* All new functionality must include appropriate unit tests
* Code must pass linting and static analysis checks
* Breaking changes must be reflected in version increments
* Unstable or experimental functionality should be clearly identified

### Developer Workflow Summary

* Create a feature branch from main
* Implement changes
* Run local tests and linting
* Submit a pull request targeting main
* Pass CI validation
* Merge into main
* Tag a release when appropriate
