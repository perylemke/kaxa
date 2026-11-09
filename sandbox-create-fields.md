# Mapa dos campos de create por provider

Este documento descreve o que o kaxa envia na **chamada de criação** de cada sandbox (API REST ou SDK), a partir do código em `internal/lifecycle` e `internal/provider/*`. Reflete o comportamento atual do repositório; não substitui a documentação oficial de cada provedor.

## Entrada comum: `lifecycle.Request` → `sandbox.Spec`

`lifecycle.createSandbox` chama `Provider.Create` com um `sandbox.Spec` montado assim:

| Campo em `sandbox.Spec` | Origem no fluxo de create |
| --- | --- |
| `Name` | Nome escolhido pelo usuário |
| `Labels` | Sempre inclui `kaxa`; com `--sync` inclui também `sync` |
| `Running` | `--running` (Sprites) |
| `ExposeAPI` | `--api` |
| `APIPort` | `--api-port` quando `--api` está ativo; senão `0` |
| `Template` | `--template` ou vazio (default do provider) |

Implementação: [`internal/lifecycle/lifecycle.go`](internal/lifecycle/lifecycle.go) (`createSandbox`).

### Campos do create que **não** vão na chamada de criação da sandbox

Estes campos de `lifecycle.Request` são tratados **depois** do `Create` (ou em providers que não os usam no create):

| Campo | Quando |
| --- | --- |
| `Sync`, `Root`, `Repo`, `NoInstall` | Cópia do repositório e install de toolchain (`syncAndInstall`) |
| `APICmd`, `APIEnv` | Publicação do serviço HTTP (`publishAPI`), exceto onde a porta/traffic já entram no create (E2B, Blaxel parcialmente) |
| `Public` | Auth da URL (`SetURLAuth`), só Sprites |

Após o create, todos os providers com workspace chamam `PrepareWorkspace` quando aplicável.

---

## Sprites (`sprites`)

**SDK:** `github.com/superfly/sprites-go` — `CreateSpriteWithOrg`  
**HTTP:** `POST {base}/v1/sprites`

### Body JSON (`CreateSpriteRequest`)

| Campo | Valor no kaxa |
| --- | --- |
| `name` | `spec.Name` |
| `config` | `nil` |
| `environment` | omitido |
| `labels` | `spec.Labels`, mais `terminal` quando `Running && !ExposeAPI` |

Organização: `org` passado como `nil` (org padrão do token).

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim → `name` |
| `Labels` | Sim → `labels` (pode ganhar `terminal`) |
| `Running` | Não no body; se true, após warm chama `configureRunning` (hold de 1h + serviço HTTP tmux/API) |
| `ExposeAPI` | Não no body; influencia `configureRunning` e labels |
| `APIPort` | Não no create |
| `Template` | Não suportado |

Código: [`internal/provider/sprites/sprites.go`](internal/provider/sprites/sprites.go), [`internal/provider/sprites/client.go`](internal/provider/sprites/client.go).

---

## Modal (`modal`)

**SDK:** `github.com/modal-labs/libmodal/modal-go` — `Sandboxes.Create`  
**App:** nome fixo `kaxa` (`Apps.FromName`, `CreateIfMissing: true`)

### `SandboxCreateParams`

| Campo | Valor no kaxa |
| --- | --- |
| `Name` | `spec.Name` |
| `Tags` | `kaxa.name` = nome; cada label em `spec.Labels` vira tag com valor `"true"` |
| `Command` | `["sleep", "infinity"]` |
| `Timeout` | 24h (máximo Modal) |
| `IdleTimeout` | 1h |
| Imagem | `debian:bookworm-slim` (digest fixo em código); se `sync` ∈ labels, imagem estendida com `syncImageCommands` (ferramentas no Dockerfile da imagem) |

`withTools` = `labels` contém `sync`.

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim |
| `Labels` | Sim → tags |
| `Running` | Não |
| `ExposeAPI` / `APIPort` | Não |
| `Template` | Não |

Código: [`internal/provider/modal/modal.go`](internal/provider/modal/modal.go), [`internal/provider/modal/client.go`](internal/provider/modal/client.go).

---

## E2B (`e2b`)

**HTTP:** `POST https://api.e2b.app/v2/sandboxes`

### Body JSON

| Campo | Valor no kaxa |
| --- | --- |
| `templateID` | `spec.Template` resolvido: flag → `KAXA_E2B_TEMPLATE` → literal `base` no JSON (template default E2B) |
| `timeout` | `3600` (segundos; pausa após idle) |
| `autoPause` | `true` |
| `network.allowPublicTraffic` | `true` só quando `spec.ExposeAPI` |
| `metadata` | Ver tabela abaixo |

### Metadados (`metadata`)

| Chave | Valor |
| --- | --- |
| `kaxa.managed` | `"true"` |
| `kaxa.name` | nome da sandbox |
| `kaxa.labels` | labels unidas por vírgula |
| `kaxa.api_port` | porta, se `ExposeAPI` |
| `kaxa.template` | template explícito, se não vazio |

Com `--api`, `CreateParams` também define `APIPort` e `PublicTraffic` antes do REST (refletidos em metadata e network).

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim → metadata |
| `Labels` | Sim → metadata |
| `Running` | Não |
| `ExposeAPI` | Sim → `allowPublicTraffic` |
| `APIPort` | Sim → metadata `kaxa.api_port` |
| `Template` | Sim → `templateID` / metadata |

Código: [`internal/provider/e2b/e2b.go`](internal/provider/e2b/e2b.go), [`internal/provider/e2b/client.go`](internal/provider/e2b/client.go).

---

## Docker Sandboxes (`docker`)

**HTTP:** `POST {base}/v1/sandboxes`  
**Header:** `Idempotency-Key` (aleatório por tentativa)

### Body JSON (`createBody`)

| Campo | Valor no kaxa |
| --- | --- |
| `displayName` | `spec.Name` |
| `imageRef` | `spec.Template` → `KAXA_DOCKER_TEMPLATE` → `docker/sandbox-templates:shell` |
| `resources.cpus` | `2` |
| `resources.memoryMib` | `4096` |
| `labels` | `kaxa.managed=true`; para cada label em `spec.Labels`, `kaxa.label.<label>=true` |
| `startupExecution.rawImage.start.argv` | `["tail", "-f", "/dev/null"]` |
| `features.timeouts.timeout` | `"3600s"` |
| `features.timeouts.onTimeout` | `"stop"` |

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim → `displayName` |
| `Labels` | Sim → `labels` |
| `Running` | Não |
| `ExposeAPI` / `APIPort` | Não no create (API publicada depois) |
| `Template` | Sim → `imageRef` |

Código: [`internal/provider/docker/docker.go`](internal/provider/docker/docker.go), [`internal/provider/docker/client.go`](internal/provider/docker/client.go).

---

## Daytona (`daytona`)

**SDK:** `github.com/daytona/clients/sdk-go` — `Client.Create`  
**Parâmetros:** `types.SnapshotParams` + `WithTimeout(2m)`, `WithWaitForStart(false)`

### `SnapshotParams` (campos setados pelo kaxa)

| Campo | Valor no kaxa |
| --- | --- |
| `Name` | `spec.Name` |
| `Snapshot` | `spec.Template` → `KAXA_DAYTONA_TEMPLATE`; se vazio, string vazia (snapshot/imagem default da plataforma Daytona) |
| `Labels` | Ver tabela |
| `NetworkBlockAll` | `spec.NetworkBlockAll` (`--block-all-network-access`). Desligado por padrão. Bloqueia só a saída |

Demais campos de `SandboxBaseParams` (`User`, `EnvVars`, `Public`, intervalos auto-stop/pause/archive/delete, `TtlMinutes`, etc.) ficam no zero value — o kaxa não os preenche. O restore de snapshot também deixa `NetworkBlockAll` desligado.

### Labels

| Chave | Valor |
| --- | --- |
| `kaxa.managed` | `"true"` |
| `kaxa.name` | nome |
| `kaxa.labels` | labels unidas por vírgula |
| `kaxa.api_port` | se `ExposeAPI` |
| `kaxa.template` | se template não vazio |

Com `--api`, `CreateParams.APIPort` = `spec.APIPort` antes do SDK.

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim |
| `Labels` | Sim |
| `Running` | Não |
| `ExposeAPI` | Indireto → label `kaxa.api_port` |
| `APIPort` | Sim → label |
| `Template` | Sim → `Snapshot` + label opcional |
| `NetworkBlockAll` | Sim → `NetworkBlockAll` |

Código: [`internal/provider/daytona/daytona.go`](internal/provider/daytona/daytona.go), [`internal/provider/daytona/client.go`](internal/provider/daytona/client.go).

---

## Blaxel (`blaxel`)

**SDK:** `github.com/blaxel-ai/sdk-go` — `SandboxService.NewInstance`  
**Body:** `SandboxNewParams` → `Sandbox` + `Metadata` + `Spec`

### Metadados e spec

| Caminho | Valor no kaxa |
| --- | --- |
| `metadata.name` | `spec.Name` |
| `metadata.labels` | `kaxa.managed`, `kaxa.name`, `kaxa.labels`, `kaxa.template` (= imagem escolhida), `kaxa.api_port` se API |
| `spec.region` | `KAXA_BLAXEL_REGION` ou `us-pdx-1` |
| `spec.runtime.image` | `spec.Template` → `KAXA_BLAXEL_TEMPLATE` → `blaxel/base-image:latest` |
| `spec.runtime.memory` | `KAXA_BLAXEL_MEMORY` (MiB) ou `4096` |
| `spec.runtime.ports` | Só com `--api`: um port HTTP, `target` = porta, `name` = `kaxa-api` |

### `sandbox.Spec` → create

| Spec | Enviado no create? |
| --- | --- |
| `Name` | Sim |
| `Labels` | Sim → metadata |
| `Running` | Não |
| `ExposeAPI` | Sim → ports + label `kaxa.api_port` |
| `APIPort` | Sim → ports + label |
| `Template` | Sim → imagem + label `kaxa.template` |

Região e memória vêm de env do provider, não de flags de create do CLI.

Código: [`internal/provider/blaxel/blaxel.go`](internal/provider/blaxel/blaxel.go), [`internal/provider/blaxel/client.go`](internal/provider/blaxel/client.go).

---

## Resumo: o que cada `sandbox.Spec` afeta no create

| Provider | Name | Labels | Running | ExposeAPI | APIPort | Template |
| --- | --- | --- | --- | --- | --- | --- |
| Sprites | body | body (+ terminal) | pós-create | pós-create | — | — |
| Modal | SDK | tags | — | — | — | — |
| E2B | metadata | metadata | — | network | metadata | templateID |
| Docker | displayName | labels | — | — | — | imageRef |
| Daytona | SDK | labels | — | label | label | Snapshot |
| Blaxel | metadata | metadata | — | ports | ports | runtime.image |

Legenda: **—** = não enviado na chamada de criação; **pós-create** = passos extras no mesmo `Create` do provider, fora do payload inicial da API/SDK.
