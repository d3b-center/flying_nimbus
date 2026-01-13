
---

# ☁️ Flying Nimbus — Phase 1

![](../image.png)
## High Level Requirements
AWS Support for the following:
* [ ] EC2
* [ ] EC2 SSM
* [ ] RDS
* [ ] Dev Tunnels
* [ ] Service Catalog

SDLC
* [ ] Unit Test
* [ ] Integration Test
* [ ] Version Strategy
* [ ] Branch Strategy Defined



## 1. Core Architecture (Foundation)

### App / Core

* [ ] Define **AppServices** 
  * Holds:
    * active provider (`aws`, `azure`, future)
    * shared config (region, profile)
    * backend provider services (AWS wrapper)
    * (First Pass Init Config for AWS )
* [ ] No direct SDK usage in TUI models
* [ ] Providers expose **capabilities**, not screens

### Navigation

* [ ] RootModel
  * [ ] Holds navigation stack (`[]tea.Model`)
  * [ ] Handles `NavigationMsg`
  * [ ] Breadcrumb rendering support (Do we even need?)
* [ ] NavigationMsg
  * [ ] Push model
  * [ ] Pop model
  * [ ] Reset stack
* [ ] Breadcrumb component (read-only, derived from stack) (Do we need for Phase 1)

### TUI Framework
* [ ] Bubble Tea
* [ ] LipGloss:
* [ ] Bubbles:
  * [ ] list
  * [ ] spinner
  * [ ] textinput
  * [ ] help
  * [ ] etc
* [ ] Standard keybindings
  * [ ] Enter = select
  * [ ] Esc = back
  * [ ] q = quit
  * [ ] ? = help

---

## 2. Provider Selection

### Providers Menu

* [ ] ProvidersModel (TUI)
  * [ ] List of providers
    * AWS
    * (Azure – placeholder)
* [ ] Provider selection initializes provider backend
* [ ] Provider-specific menu pushed onto stack
* [ ] Show visually the account and profile (somewhere)

---

## 3. AWS Provider (Backend)

### AWS Service Wrapper
* [ ] `AWSService` abstraction
  * [ ] EC2
  * [ ] SSM
  * [ ] RDS
  * [ ] Service Catalog
* [ ] Configurable via:
  * [ ] AWS profile
  * [ ] Region
* [ ] Context-aware (cancelable operations)
* [ ] Error wrapping with user-friendly messages

---

## 4. AWS Provider Menu (TUI)

### AWS Provider Menu
* [ ] EC2
* [ ] EC2 SSM
* [ ] RDS
* [ ] Dev Tunnels
* [ ] Service Catalog

This is a **provider-level submenu**, not root.

---

## 5. EC2 Management
### EC2 List
* [ ] List EC2 instances
  * [ ] Name
  * [ ] Instance ID
  * [ ] State
  * [ ] Type
* [ ] Loading state
* [ ] Error handling

### EC2 Actions
* [ ] Start instance
* [ ] Stop instance
* [ ] Refresh list

---

## 6. EC2 SSM

### SSM Capability
* [ ] Detect SSM-managed instances
* [ ] Start SSM session
* [ ] Show connection status
* [ ] Graceful disconnect

---

## 7. RDS

### RDS List

* [ ] List available databases
  * [ ] Identifier
  * [ ] Engine
  * [ ] Status
  * [ ] Endpoint (masked)
* [ ] Filter by engine
* [ ] Refresh support

---

## 8. Dev Tunnels 

### Dev Tunnel Management
* [ ] Create N concurrent dev tunnels
* [ ] Select RDS DB as tunnel target
* [ ] Choose:
  * [ ] Local port
  * [ ] Remote port
* [ ] Tunnel runs in background
* [ ] Persist tunnel metadata

### Tunnel Lifecycle
* [ ] Start tunnel
* [ ] Stop tunnel
* [ ] List active tunnels
* [ ] Auto-reconnect (optional phase 2)

---

## 9. Service Catalog

### Service Catalog List

* [ ] List available products
* [ ] Show:
  * [ ] Name
  * [ ] Version
  * [ ] Status

### Actions
* [ ] Provision product
* [ ] Terminate product
* [ ] Show provisioning progress

---

## 10. UX / DX

### UX

* [ ] Consistent loading indicators
* [ ] Clear error messages
* [ ] Keyboard-only navigation

### DX

* [ ] Clean separation:
  * TUI → App/Core → Provider Backend
* [ ] Provider-agnostic navigation logic
* [ ] Easy to add new providers

---

## 11. CI / CD & Quality Gates

### Continuous Integration (CI)

* [ ] CI must pass:
  * build
  * lint
  * unit tests

---
