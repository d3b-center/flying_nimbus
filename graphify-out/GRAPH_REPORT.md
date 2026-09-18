# Graph Report - .  (2026-08-05)

## Corpus Check
- Corpus is ~47,352 words - fits in a single context window. You may not need a graph.

## Summary
- 898 nodes · 1894 edges · 38 communities (33 shown, 5 thin omitted)
- Extraction: 95% EXTRACTED · 5% INFERRED · 0% AMBIGUOUS · INFERRED: 99 edges (avg confidence: 0.82)
- Token cost: 1,200 input · 380 output

## Community Hubs (Navigation)
- Service Catalog Views
- RDS and DevOps Commands
- Security Group Layer
- Core AWS Services
- TUI Components
- Bedrock AI Services
- DevOps Agent Chat
- EC2 Instance Views
- Input Form Components
- EC2 Data Models
- Secrets and Parameter Store
- CloudWatch and MWAA
- CI/CD Workflows
- ECS Container Logs
- EC2 Mock Tests
- RDS Service and Tests
- Step Functions
- Action Menu UI
- Secrets Manager
- Parameter Store Service
- Navigation Models
- IAM Users and Keys
- App Bootstrap
- AWS Provider Model
- Security Group Tests
- Providers Model
- AWS Provider Init
- CLI Entry Point
- Input Routing Strategy
- Bedrock Mock Tests
- Log Buffer Utility
- Key-Value Config
- Dragon Ball Inspiration
- Identity Tests
- Common Tests
- Keymap Constants
- Release Changelog
- Go Module Root

## God Nodes (most connected - your core abstractions)
1. `ServiceCatalogViewModel` - 44 edges
2. `App` - 36 edges
3. `DevOpsAgentViewModel` - 32 edges
4. `Ec2ViewModel` - 27 edges
5. `DevOpsAgentViewModel` - 26 edges
6. `newSvc()` - 25 edges
7. `RdsViewModel` - 25 edges
8. `RootModel` - 25 edges
9. `ContentWindowSizeMsg` - 23 edges
10. `Ec2Service` - 22 edges

## Surprising Connections (you probably didn't know these)
- `AWS Provider (auto-grabs AWS_PROFILE and AWS_REGION, validates credentials at startup)` --semantically_similar_to--> `IAM Role Assumption (minimal permissions security pattern)`  [INFERRED] [semantically similar]
  docs/design/phase1.md → README.md
- `Run()` --calls--> `InitApp()`  [INFERRED]
  cmd/root.go → internal/app/app.go
- `Run()` --calls--> `StartTea()`  [INFERRED]
  cmd/root.go → internal/tui/tui.go
- `Create Release Job (make release, tag, upload binaries)` --conceptually_related_to--> `Release v1.2.0 (default values, input verification)`  [INFERRED]
  .github/workflows/release.yml → CHANGELOG.md
- `main()` --calls--> `Execute()`  [INFERRED]
  main.go → cmd/root.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Phase 1 Core AWS Capabilities** — docs_design_phase1_ec2_management, docs_design_phase1_rds_feature, docs_design_phase1_service_catalog_feature, docs_design_phase1_dev_tunnel [EXTRACTED 1.00]
- **CI Pipeline Validation Jobs** — _github_workflows_ci_lint_job, _github_workflows_ci_build_job, _github_workflows_ci_test_job [EXTRACTED 1.00]
- **Developer Workflow System (branch strategy + CI validation + release automation)** — docs_design_phase1_branch_strategy, _github_workflows_ci_ci_workflow, _github_workflows_release_release_fn_workflow, docs_design_phase1_release_process [INFERRED 0.85]

## Communities (38 total, 5 thin omitted)

### Community 0 - "Service Catalog Views"
Cohesion: 0.06
Nodes (39): Product, ProvisionedProduct, ProvisioningArtifact, ProvisioningParameter, serviceCatalogAPI, ServiceCatalogService, Config, Context (+31 more)

### Community 1 - "RDS and DevOps Commands"
Cohesion: 0.07
Nodes (40): RDSInstance, devOpsHelpText(), ecsLogsSuggestions(), ecsLsSuggestions(), eventLabel(), expandRefTokens(), expandResourceRefs(), fetchECSServicesCmd() (+32 more)

### Community 2 - "Security Group Layer"
Cohesion: 0.07
Nodes (36): SecurityGroup, SecurityGroupRule, sgAPI, SgService, SectionRowsFn, flattenIpPermissions(), Config, Context (+28 more)

### Community 3 - "Core AWS Services"
Cohesion: 0.06
Nodes (37): AwsService, CallerIdentity, cfnAPI, CloudFormationService, emrAPI, EmrCluster, EmrService, EmrStep (+29 more)

### Community 4 - "TUI Components"
Cohesion: 0.08
Nodes (22): Attr, Commands, WindowSizeMsg, RenderErrorModal(), RenderModal(), RenderDevConsole(), GenerateNimbusKeyMap(), Binding (+14 more)

### Community 5 - "Bedrock AI Services"
Cohesion: 0.09
Nodes (40): bedrockAPI, BedrockModel, BedrockService, BedrockToken, Config, Context, Duration, Time (+32 more)

### Community 6 - "DevOps Agent Chat"
Cohesion: 0.08
Nodes (22): AgentSpace, devopsAgentAPI, DevOpsAgentChatResult, DevOpsAgentService, Config, Context, InitDevOpsAgentService(), Cmd (+14 more)

### Community 7 - "EC2 Instance Views"
Cohesion: 0.09
Nodes (25): GenerateEbsVolumeRows(), GenerateTagRows(), ec2InstancesToItems(), fetchEc2InstancesCmd(), generateEc2InstanceDetail(), Cmd, Context, Item (+17 more)

### Community 8 - "Input Form Components"
Cohesion: 0.11
Nodes (23): InputField, InputForm, InputFormCancelMsg, InputFormOpenMsg, InputFormResult, InputFormSubmitMsg, Cmd, KeyMsg (+15 more)

### Community 9 - "EC2 Data Models"
Cohesion: 0.11
Nodes (19): EbsVolume, ec2API, Ec2Instance, Ec2Service, GroupIdentifier, IamInstanceProfile, InstanceBlockDeviceMapping, extractIamProfile() (+11 more)

### Community 10 - "Secrets and Parameter Store"
Cohesion: 0.14
Nodes (16): fetchAllSecretsCmd(), fetchParameterStoreCmd(), fetchSecretFieldsCmd(), fetchSecretsManagerCmd(), Cmd, Context, Item, KeyMsg (+8 more)

### Community 11 - "CloudWatch and MWAA"
Cohesion: 0.15
Nodes (15): cwlAPI, mwaaAPI, MWAADag, MWAADagRun, MWAAEnvironment, MWAAService, MWAATaskInstance, Client (+7 more)

### Community 12 - "CI/CD Workflows"
Cohesion: 0.09
Nodes (26): Build Job (make build), CI Workflow, Lint Job (gofmt check), Test Job (make test), Release Binary: flying-nimbus-linux-amd64, Release Binary: flying-nimbus-mac-arm64, Create Release Job (make release, tag, upload binaries), Release FN Workflow (+18 more)

### Community 13 - "ECS Container Logs"
Cohesion: 0.14
Nodes (18): ContainerLogs, CWLEvent, EcsCluster, ecsCWLAPI, ecsListAPI, EcsService, EcsServiceSummary, EcsTaskSummary (+10 more)

### Community 14 - "EC2 Mock Tests"
Cohesion: 0.13
Nodes (19): mockEc2API, DescribeInstancesOutput, DescribeVolumesInput, DescribeVolumesOutput, Context, DescribeInstancesInput, T, TestEc2Instance_ListItemInterface() (+11 more)

### Community 15 - "RDS Service and Tests"
Cohesion: 0.12
Nodes (19): mockRdsAPI, rdsAPI, RdsService, DBSubnetGroup, DescribeDBInstancesInput, DescribeDBInstancesOutput, getSecurityGroupIds(), getVpcIdAndSubnetIds() (+11 more)

### Community 16 - "Step Functions"
Cohesion: 0.17
Nodes (17): sfnAPI, SfnExecution, SfnLogEvent, SfnService, StateMachine, HistoryEventType, buildExecutionArn(), findStateMachineInList() (+9 more)

### Community 17 - "Action Menu UI"
Cohesion: 0.16
Nodes (15): ActionItem, ActionMenu, ModalCancelMsg, ModalResponseMsg, Cmd, KeyMsg, Msg, NewActionModal() (+7 more)

### Community 18 - "Secrets Manager"
Cohesion: 0.18
Nodes (11): Secret, secretsManagerAPI, SecretsService, extractSecretTags(), formatSecretTime(), Config, Context, Tag (+3 more)

### Community 19 - "Parameter Store Service"
Cohesion: 0.20
Nodes (9): Parameter, parameterStoreAPI, ParameterStoreService, DescribeParametersInput, formatParameterTime(), Config, Context, Time (+1 more)

### Community 20 - "Navigation Models"
Cohesion: 0.22
Nodes (8): BackMsg, Commanded, NavigateMsg, NavItem, NimbusModel, Titled, NewNavItem(), Model

### Community 21 - "IAM Users and Keys"
Cohesion: 0.32
Nodes (8): AccessKey, iamAPI, IamService, IAMUser, Config, Context, Time, InitIamService()

### Community 22 - "App Bootstrap"
Cohesion: 0.29
Nodes (8): App, CancelFunc, Context, InitApp(), InitLogger(), InitRoot(), StartTea(), Logger

### Community 23 - "AWS Provider Model"
Cohesion: 0.24
Nodes (4): AwsProviderModel, Cmd, Model, Msg

### Community 24 - "Security Group Tests"
Cohesion: 0.27
Nodes (8): mockSgAPI, DescribeSecurityGroupsInput, DescribeSecurityGroupsOutput, Context, T, TestDescribeSecurityGroupRules(), TestDescribeSecurityGroupRules_Error(), TestFlattenIpPermissions()

### Community 25 - "Providers Model"
Cohesion: 0.27
Nodes (5): Cmd, Model, Msg, NewProvidersModel(), ProvidersModel

### Community 26 - "AWS Provider Init"
Cohesion: 0.42
Nodes (7): ContentWindowSizeMsg, NewAWSProviderModel(), InitDevOpsAgentViewModel(), InitEc2ViewModel(), InitRdsViewModel(), InitSecretsViewModel(), InitServiceCatalogViewModel()

### Community 27 - "CLI Entry Point"
Cohesion: 0.29
Nodes (4): Execute(), PrintVersion(), Run(), main()

### Community 29 - "Bedrock Mock Tests"
Cohesion: 0.36
Nodes (6): mockBedrockAPI, Context, ListFoundationModelsInput, ListFoundationModelsOutput, ListInferenceProfilesInput, ListInferenceProfilesOutput

### Community 30 - "Log Buffer Utility"
Cohesion: 0.38
Nodes (3): LogBuffer, NewLogBuffer(), RWMutex

### Community 31 - "Key-Value Config"
Cohesion: 0.52
Nodes (6): kvConfig, KVOption, Style, WithLabelStyle(), WithLabelWidth(), WithValueStyle()

### Community 32 - "Dragon Ball Inspiration"
Cohesion: 1.00
Nodes (4): Dragon Ball (Franchise/Series), Flying Nimbus (Kinto-Un Cloud), Goku (Dragon Ball Character), Goku on Flying Nimbus - Minimalist Wallpaper

### Community 33 - "Identity Tests"
Cohesion: 0.67
Nodes (3): T, TestCallerIdentity_parseARN(), TestCallerIdentity_WhoAmI()

## Knowledge Gaps
- **23 isolated node(s):** `flying_nimbus`, `SsmSessionType`, `SsmInstanceStatus`, `ModalCancelMsg`, `ModalResponseMsg` (+18 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `App` connect `App Bootstrap` to `Service Catalog Views`, `RDS and DevOps Commands`, `Security Group Layer`, `Core AWS Services`, `TUI Components`, `DevOps Agent Chat`, `EC2 Instance Views`, `Secrets and Parameter Store`, `Navigation Models`, `AWS Provider Model`, `Providers Model`, `AWS Provider Init`, `CLI Entry Point`, `Log Buffer Utility`?**
  _High betweenness centrality (0.351) - this node is a cross-community bridge._
- **Why does `AwsService` connect `Core AWS Services` to `Service Catalog Views`, `Security Group Layer`, `Bedrock AI Services`, `DevOps Agent Chat`, `EC2 Data Models`, `CloudWatch and MWAA`, `ECS Container Logs`, `RDS Service and Tests`, `Step Functions`, `Secrets Manager`, `Parameter Store Service`, `IAM Users and Keys`, `App Bootstrap`?**
  _High betweenness centrality (0.341) - this node is a cross-community bridge._
- **Why does `BedrockService` connect `Bedrock AI Services` to `Core AWS Services`?**
  _High betweenness centrality (0.109) - this node is a cross-community bridge._
- **What connects `flying_nimbus`, `SsmSessionType`, `SsmInstanceStatus` to the rest of the system?**
  _23 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Service Catalog Views` be split into smaller, more focused modules?**
  _Cohesion score 0.055600106923282544 - nodes in this community are weakly interconnected._
- **Should `RDS and DevOps Commands` be split into smaller, more focused modules?**
  _Cohesion score 0.06905370843989769 - nodes in this community are weakly interconnected._
- **Should `Security Group Layer` be split into smaller, more focused modules?**
  _Cohesion score 0.06936026936026936 - nodes in this community are weakly interconnected._