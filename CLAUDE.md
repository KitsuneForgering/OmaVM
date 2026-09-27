# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Este arquivo é a constituição técnica e de produto do OmaVM. Qualquer agente (humano ou IA) que trabalhe neste repositório deve segui-lo. Quando uma decisão de código conflitar com estas regras, resolva a favor destas regras ou documente o conflito explicitamente antes de prosseguir.

## Status do repositório

Neste momento o repositório contém apenas `README.md`, `LICENSE` e este `CLAUDE.md` — não há código-fonte, build system nem testes ainda. Não existe, portanto, nenhuma arquitetura legada com a qual este documento possa conflitar. As seções abaixo definem a arquitetura-alvo para quando a implementação começar; nenhuma delas deve ser tratada como já implementada.

**Linguagem: Go**, confirmado. Além disso, siga apenas as convenções idiomáticas padrão do Go (formatação via `gofmt`, nomes de pacote, etc.) — o layout de diretórios em si é uma decisão aberta (ver Open Technical Decisions).

**GUI toolkit: Qt 6 + Qt Quick/QML**, confirmado, no mesmo stack de OMAcut, OMAwrite, OMAcalc e Quickshell. Use Qt Quick Controls com Material, leia `~/.local/state/omarchy/current/theme/colors.toml`, acompanhe sua troca e respeite dark/light, accent, background, foreground e escala de texto. Não crie um sistema de temas próprio. O layout deve refluir durante resize contínuo de janelas tiled no Hyprland e nunca depender de tamanhos fixos para monitores específicos.

**Box backend: Distrobox sobre Podman (preferencial) ou Docker**, confirmado. O Distrobox fornece a integração madura com HOME, Wayland/X11, áudio, dispositivos e aplicações gráficas; a OmaVM fornece o modelo de produto e lifecycle unificado. Containers diretos ficam reservados a ambientes Disposable e à compatibilidade com Boxes legadas. Ver Backend Rules.

## Project Mission

> OmaVM é uma experiência integrada para criar, executar e administrar ambientes no Omarchy.

OmaVM é um gerenciador de **ambientes computacionais** para o ecossistema Omarchy — não apenas um frontend para QEMU/Distrobox e não um hypervisor ou container runtime próprio. O valor do produto está no modelo conceitual, na UX e na orquestração coerente dessas ferramentas maduras.

## Product Model

A abstração central do produto:

```text
Environment
├── Box       (userspace Linux compartilhando o kernel do host)
└── Machine   (kernel independente)
```

O usuário pensa em "quero um ambiente Fedora", nunca em "quero um container Podman" ou "quero uma VM QEMU". Backend é um detalhe de implementação escondido do usuário no fluxo básico.

## Box vs Machine

Regra de decisão:

```text
Linux userspace only  -> Box     (backend: Distrobox sobre Podman/Docker)
Independent kernel     -> Machine (backend: QEMU/KVM)
```

Regra **proibida**: `Linux = container, non-Linux = VM`. Linux pode e deve rodar como `Machine` quando o usuário precisar de kernel próprio, boot completo, init/systemd independente, testes de kernel, networking completo ou isolamento de VM (ex.: testes de kernel Linux, boot do Omarchy, Arch com kernel custom).

Exemplos:

| Caso | Kind |
|---|---|
| Ubuntu/Fedora/Debian/Arch/Alpine userspace | Box |
| Arch com kernel custom | Machine |
| Testes de kernel Linux / boot do Omarchy | Machine |
| FreeBSD, OpenBSD, Haiku, ISO customizada | Machine |

Boxes e Machines compartilham conceitos de produto (criar, iniciar, parar, snapshot, clone, exec) mas suas diferenças de isolamento **nunca** devem ser escondidas quando forem semanticamente relevantes (ver Security Model).

**Nota de pesquisa (2026-09-26)**: o Parallels — nossa referência filosófica de UX — só gerencia um tipo de objeto (VM); ele nunca precisa resolver "qual kind de isolamento". A decisão Box-vs-Machine é um problema de UX que a OmaVM tem que validar por conta própria, sem precedente direto para copiar do Parallels.

## Architecture

```text
             GUI
              │
CLI ───── OmaVM Core (domínio)
              │
         Backend Layer (adapters)
          /                    \
  Development Box          Machine backend
  (Distrobox -> Podman/    (QEMU/KVM)
   Docker)
```

- **Core/domínio**: modela `Environment`, `Box`, `Machine`, `Image`, `Template`, `Snapshot`, `Project`, `Integration`, `Backend`. Não depende de ferramentas externas específicas. O frontend QML acessa esse Core pelo contrato JSON estável do CLI via `QProcess`, sempre com programa e argumentos separados (nunca por shell).
- **Backend layer**: interface própria do OmaVM (não é a API do Distrobox/Podman/Docker nem do libvirt/QEMU exposta diretamente). Deve permitir adicionar backends futuros (Incus, Remote, Cloud) sem contaminar o domínio — mas não construa essas abstrações antes de existir necessidade real.
- Detalhes de infraestrutura (`distrobox`, `podman`, `docker`, `qemu-system-x86_64`, `virsh`, caminhos de qcow2 e comandos específicos) ficam confinados aos adapters/backends e nunca vazam para UI, CLI de alto nível ou domínio.
- Modelo conceitual de referência (não é API obrigatória — preserve/evolua o que já existir no código):

```go
type EnvironmentKind int
const (
    Box EnvironmentKind = iota
    Machine
)

type Environment struct {
    ID, Name, Image, Backend string
    Kind EnvironmentKind
}

type Backend interface {
    Create(...); Start(...); Open(...); Stop(...)
    Status(...); Exec(...); Remove(...)
}
```

O conjunto mínimo de operações do `Backend` é `Create, Start/Open, Stop, Status, Exec, Remove` (ver Current Priorities). A assinatura exata de cada método ainda não está definida — decidir seguindo idiomas padrão de Go (contextos explícitos via `context.Context`, erros estruturados, sem pânico em caminho normal) quando a implementação começar.

### Daemon (`omavmd`)

Não introduza um daemon "porque gerenciadores de VM têm daemon". `omavmd` só se justifica quando houver necessidade concreta de lifecycle persistente, eventos, background jobs, comunicação com guest, monitoring ou automation.

### OmaVM Guest (futuro)

Componente guest opcional para Machines (`omavm-guest`, comunicando via virtio/vsock com `omavmd`) para status, clipboard, notifications, resolução dinâmica, file transfer, exec, shutdown/reboot. Não implementar antes de uma funcionalidade concreta exigir. Boxes não devem depender de `omavm-guest` quando o backend já resolve a integração diretamente.

### Blend Mode (futuro, equivalente ao Coherence Mode)

Meta de longo prazo: aplicações de um Environment aparecem como janelas normais do Omarchy. Não construir prematuramente, mas não tomar decisões arquiteturais que tornem isso impossível depois.

**Lição de pesquisa (Coherence Mode do Parallels, 2026-09-26)**: usuários reclamam de indicadores de sistema duplicados (bateria, rede, relógio) quando a bandeja do guest é fundida à barra do host. Quando Blend Mode for implementado de fato: indicadores de sistema do guest que o host **já mostra** não devem reaparecer — fundir aplicações, não duplicar chrome de sistema.

## Backend Rules

- Development Boxes usam Distrobox via adapter próprio da OmaVM. Distrobox escolhe Podman ou Docker como container manager e é responsável pela integração madura com o host; não replique essa integração no Core.
- Containers diretos sobre Podman/Docker são um backend distinto, reservado ao futuro modo Disposable (CI local, agentes e tarefas efêmeras) e à compatibilidade de lifecycle com Boxes criadas antes desta decisão. Novas Development Boxes nunca devem cair silenciosamente no adapter legado quando Distrobox estiver ausente: falhe com instrução clara de instalação.
- A UI básica nunca pergunta por QEMU, Distrobox, Podman ou Docker. Ela pergunta pela intenção: Desktop, Development Box ou, quando implementado, Disposable.
- Machines usam QEMU/KVM via adapter.
- Antes de construir infraestrutura nova, pergunte: **"isso é parte da experiência exclusiva do OmaVM, ou já é resolvido pelo backend?"**. Se já resolvido por Podman/Docker/QEMU/KVM/virtio/ferramentas maduras, integre — não reimplemente o motor subjacente.

## UX Principles

Referência filosófica: **Parallels Desktop** — não visualmente, mas na ideia de que "virtualização deve desaparecer atrás da experiência". OmaVM não deve parecer um `virt-manager` estilizado.

1. Criar um environment não deve exigir conhecimento de virtualização.
2. Linux userspace deve preferir Box; kernel independente deve usar Machine.
3. O usuário não precisa entender QEMU, KVM, qcow2, Podman, Docker, bridges, sockets, virtio, vsock, namespaces para criar/usar um ambiente — esses detalhes vivem em áreas avançadas/diagnóstico.
4. Backend não deve dominar a interface: nunca pergunte "qual backend deseja usar?" — pergunte "o que você quer executar?" (ex.: escolher a distro, e para Linux escolher entre "Development Environment" → Box ou "Virtual Machine" → Machine; para sistemas não-Linux, `Machine` é selecionado automaticamente).
   Na implementação Qt atual, essa decisão aparece primeiro como **Desktop** (Machine, exige uma ISO x86_64 e abre display gráfico) ou **Development Box** (Distrobox sobre imagem OCI, abre terminal e pode executar/exportar aplicações gráficas integradas). Nunca passe uma referência OCI como `fedora:latest` ao QEMU como se fosse mídia de boot. Box não é um desktop com kernel próprio, embora possa abrir aplicações gráficas.
5. Tela principal = **Experience Center**, um control center onde ambientes aparecem como objetos simples (nome, distro, kind, status) com ações: Open, Start, Stop, Clone, Snapshot, Settings, Delete. Detalhes de infraestrutura não pertencem a essa tela. Se agrupamento/reordenação de ambientes for implementado algum dia, deve funcionar de verdade (arrastar-e-soltar persistente, grupos reais) — o Control Center do Parallels é criticado por usuários justamente por ter listagem que não reordena manualmente nem agrupa (pesquisa de 2026-09-26); não é meta atual, só uma armadilha a evitar se/quando for construído.
6. Um Environment aberto deve parecer parte do Omarchy.
7. Configurações avançadas existem sem contaminar o fluxo básico; defaults devem ser seguros e razoáveis.
8. OmaVM deve parecer mais próximo do Parallels em simplicidade do que do virt-manager em exposição de infraestrutura.
9. Snapshots são apresentados por significado ("Before system upgrade", "Clean installation"), nunca por IDs internos; detalhes de backing file/qcow2 ficam no backend. **Quando implementado** (hoje é "snapshots sofisticados", fora do escopo da Fase 1): usar verbos de usuário como "Go To" em vez de jargão tipo "revert/commit"; ter uma view dedicada (não misturada nas Settings); considerar um limite de histórico com descarte automático do mais antigo em vez de crescimento ilimitado — padrão validado externamente no Snapshot Manager do Parallels (pesquisa de 2026-09-26).
10. Clone (linked/full) e outras operações mostram comportamento e custo ao usuário, não termos internos sem explicação.

## CLI/GUI Contract

- Toda operação importante tem representação programática. A GUI **não** contém lógica exclusiva de gerenciamento de ambientes — ela usa o CLI `omavm`, que chama o mesmo OmaVM Core. Essa fronteira entre Qt/C++ e Go é intencional; não duplique o domínio em C++.
- Nunca acoplar GUI diretamente a comandos shell/infra.
- Exemplos futuros de CLI (não implementar tudo agora, apenas manter compatível):

```bash
omavm create
omavm start radic
omavm stop radic
omavm open radic
omavm exec radic -- go test ./...
omavm ssh radic
omavm snapshot radic
omavm clone radic
omavm run arch --ephemeral
omavm status radic --json
```

- Considerar structured output (`--json`) na CLI quando fizer sentido para consumo por agentes/automação.

## Omarchy Integration

OmaVM é **Omarchy-first**. Integração com o host é feature central, não avançada. Tratar como capacidades de primeira classe (quando aplicável ao backend): clipboard, compartilhamento de diretórios, project folders, Wayland, áudio, microfone, notificações, resolução dinâmica, SSH, execução de comandos, transferência de arquivos, GPU acceleration. A UI expõe capacidades/intenções, não os mecanismos internos usados para implementá-las.

Implementado até agora: frontend Qt Quick/QML com toast in-app; entrada `.desktop` + ícone (`data/`) para o `omavm-gui` aparecer no launcher/taskbar com app-id próprio; sincronização direta com o tema Omarchy ativo (dark/light, background, foreground, accent e selection), acompanhando trocas em runtime. A aplicação apenas consome `colors.toml`; não mantém tema próprio. Como OMAcut/OMAwrite, a janela principal é tiled por padrão; não adicione regra Hyprland para fazê-la flutuar. OMAcalc é uma exceção compacta explicitamente marcada como floating, não um precedente para a OmaVM.

**Quickshell (`omarchy-shell`)**: `contrib/dev.omavm.bar` é um plugin `bar-widget` real para o shell Quickshell plugin-based do Omarchy (`/usr/share/omarchy/shell`), não especulativo — mostra a contagem de Environments (via `omavm list --json`) e abre o `omavm-gui` ao clicar. Instalado via `make install-quickshell-plugin`, mas **nunca habilitado automaticamente**: plugins de terceiros rodam sem sandbox dentro do shell já ativo do usuário, então habilitar é sempre um passo manual e explícito do usuário (`omarchy plugin enable`), nunca algo que um agente/instalador faz sozinho. Isso desbloqueou `omavm list`/`status --json` na CLI (já citado como aspiração no CLI/GUI Contract) — mantenha esse contrato JSON estável, é consumido por automação externa agora, não só hipoteticamente.

**Ideias validadas externamente para integração futura** (pesquisa sobre Parallels Desktop, 2026-09-26 — não implementar antes de necessidade real, ver Non-Goals/Current Priorities): sincronizar cor/tag de um Environment com o gerenciador de arquivos do host (Parallels sincroniza cores de VM com tags do Finder); automação ambiental tipo "Travel Mode" — ex. uma Machine rodando num laptop na bateria reduz rede/recursos automaticamente, sem exigir toggle manual do usuário.

**Logs**: ambos os binários usam `log/slog` (JSON). `/var/log` é root-owned por padrão; a OmaVM nunca eleva privilégio silenciosamente para escrever lá (ver Security Model). O destino é `/var/log/omavm/<component>.log` apenas se esse diretório já existir e for gravável; caso contrário cai para `$XDG_STATE_HOME/omavm/logs/`. Ver README seção "Logs" para o passo opcional (`sudo install -d ...`) que provisiona `/var/log/omavm`.

## Coding Principles

Preferir:
- componentes pequenos, interfaces explícitas, adapters nas bordas;
- domínio independente de ferramentas externas;
- composição em vez de herança/hierarquias;
- erros estruturados;
- operações idempotentes quando possível;
- comportamento testável sem precisar iniciar VMs/containers reais sempre que razoável;
- structured/machine-readable output em vez de parsing de output humano quando o backend oferecer.

Evitar:
- abstrações genéricas sem caso de uso real (especialmente multi-backend especulativo);
- hierarquias enormes, frameworks internos, dependências desnecessárias;
- shell commands espalhados pelo código (confinar aos adapters);
- estado global;
- lógica de domínio dentro da UI;
- duplicar funcionalidade madura de QEMU, Podman/Docker (o motor de containers em si) ou outras engines.

## Testing Strategy

- Lógica de domínio (Environment, Box, Machine, Template, etc.) deve ser testável sem depender de QEMU/Podman/Docker reais — isolar via a interface `Backend` e usar fakes/mocks nos testes de domínio e de core.
- Testes de adapter (container engine, QEMU/KVM) validam a integração real e podem depender do ambiente/host; devem ficar claramente separados dos testes de domínio. A forma exata dessa separação é uma decisão aberta (ver Open Technical Decisions) e não deve ser fixada incidentalmente.
- CLI deve ser testável invocando o Core diretamente, sem precisar de GUI.
- Baseline padrão de Go a manter assim que houver `go.mod`: `go build ./...`, `go test ./...` (com `-run <TestName>` para um teste único), `go vet ./...`, `gofmt -l .`. Comandos mais específicos (lint adicional, tags de build para testes de integração, etc.) devem ser adicionados a este arquivo assim que existirem de fato no repositório — não documentar ferramentas ainda não escolhidas.
- `Makefile` disponível como atalho para esses mesmos comandos (`make build`, `make test`, `make vet`, `make fmt-check`, `make check`); ele não substitui os comandos Go diretos, apenas os agrupa.
- CI (`.github/workflows/ci.yml`, GitHub Actions) roda `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` e compila a GUI com qmake6 em toda push/PR para `master`. Ele instala Qt 6 Base/Declarative para o frontend.

## Security Model

- Box e Machine têm garantias de isolamento diferentes e isso deve ser sempre comunicado com clareza, nunca escondido:

```text
Box     -> compartilha o kernel do host (isolamento de processo/namespace, não de kernel).
Machine -> executa um kernel independente (isolamento de VM).
```

- Nunca apresentar um Box como se oferecesse isolamento equivalente a uma VM.
- Evitar promessas genéricas como "secure sandbox" sem especificar a garantia real por trás.
- Operações privilegiadas (ex.: acesso a `/dev/kvm`, elevação de permissão) devem ser minimizadas e explícitas no código e na UX.

## Performance

- Boxes devem permanecer leves: não introduzir serviços persistentes, cópias desnecessárias ou processos pesados que anulem a vantagem de não ter um kernel próprio.
- Machines devem usar os mecanismos eficientes já oferecidos por KVM/virtio/qcow2 em vez de reimplementar equivalentes.

## Scope / Non-Goals

OmaVM **não é**:
- uma distribuição Linux;
- um fork do Distrobox;
- um container runtime;
- um hypervisor próprio;
- um substituto para QEMU;
- uma implementação de KVM;
- uma ferramenta exclusiva para Windows;
- um painel genérico de infraestrutura.

Features explicitamente fora do escopo até que uma necessidade real exista: Blend Mode completo, guest agent completo, GPU passthrough, orchestration distribuída, cloud, remote hosts, marketplace, dezenas de distros pré-empacotadas, networking editor avançado, snapshots sofisticados, plugin system.

## Development Workflow

Ao implementar qualquer feature, seguir esta ordem:

1. identificar o comportamento que o usuário precisa;
2. modelar esse comportamento no domínio;
3. determinar qual backend é responsável pela execução;
4. implementar o adapter;
5. expor no Core;
6. expor na CLI;
7. expor na GUI, quando aplicável;
8. adicionar testes;
9. atualizar documentação.

Nunca começar pela GUI chamando comandos de infraestrutura diretamente.

## Current Priorities

Fase 1 — vertical slice mínimo:

**Development Box (via Distrobox sobre Podman/Docker):** Create, Start/Open, Stop, Status, Exec, Remove. O adapter direto anterior permanece apenas para lifecycle de Environments cujo campo `backend` já seja `podman` ou `docker`.

**Machine (via QEMU/KVM):** Create, Start, Stop, Status, Open, Remove.

**Implementado (2026-09-26)**: `Open` numa Machine abre de fato um userspace gráfico via **VNC**, headless — QEMU expõe `virtio-vga-gl` sobre `-display egl-headless` (GPU acelerada mesmo sem display local) e escuta num socket unix (`-vnc unix:<state dir>/vnc.sock`, mesmo modelo de confiança local-only que o antigo socket SPICE). `Open` não chama mais um visualizador externo: lança o próprio `omavm-gui` em modo `--viewer <socket> --title <nome>`, que carrega `gui/Viewer.qml` num `Window` fullscreen falhando de forma explícita — nunca em silêncio — se `omavm-gui` não estiver instalado. O protocolo RFB é implementado do zero em `gui/vncclient.h/.cpp` (handshake 3.8, segurança "None", só encoding Raw) em vez de linkar `libvncserver`/`libvncclient`: essa lib é GPL-2.0 e o projeto é MIT, e para um socket local sem autenticação nem compressão o protocolo necessário é pequeno o suficiente pra não justificar a dependência. `gui/vncview.h/.cpp` (`QQuickPaintedItem`) desenha o framebuffer e encaminha mouse/teclado de volta (PointerEvent/KeyEvent) — é um viewer interativo, não só uma prévia.

Trade-off aceito ao sair de SPICE: perdeu-se o canal `spicevmc`/vdagent (clipboard automático host↔guest, resize dinâmico de resolução). RFB tem sua própria extensão de clipboard (`ClientCutText`/`ServerCutText`) mas isso fica para depois, não construído especulativamente — é exatamente o tipo de feature adjacente a guest agent que a seção abaixo marca como fora do escopo até haver necessidade real. Áudio (`virtio-sound-pci`/pipewire), virtiofs (shared folders) e o canal QGA (`org.qemu.guest_agent.0`, usado só para `guest-ping`/`Integration`) são independentes do protocolo de display e não mudaram.

O posicionamento da janela do viewer (fullscreen, workspace dedicado) é feito por uma regra Hyprland opt-in (`contrib/hypr/omavm-viewer.lua`, `o.window("dev.omavm.viewer", { workspace = "special:omavm", fullscreen = true })`) — nunca por `hyprctl dispatch` em runtime: a build de Hyprland em uso configura via Lua (`hl.window_rule`/`hl.dispatch`) e dispatches brutos com seletores tipo `class:...` quebram nesse parser. Como o plugin Quickshell, essa regra não é injetada automaticamente no config do usuário — é um `require` manual documentado no README.

Existe também `Previewer` (`internal/core/backend.go`), uma capability **opcional** do `Backend` (não faz parte da interface mínima acima): a Machine implementa via QMP `screendump` (independente do protocolo de display); Box não implementa — não tem display, e isso não deve ser escondido atrás de um placeholder que finja equivalência (Security Model). A GUI é o frontend Qt Quick/QML em `gui/` (`gui/EnvironmentCard.qml` mostra a miniatura por card só quando o Backend a oferece) — o antigo binding Go em `internal/gui/` foi removido.

Não implementar ainda: Blend, guest agent completo, GPU passthrough, orchestration distribuída, cloud/remote hosts, marketplace, dezenas de distros, networking editor avançado, snapshots sofisticados, plugin system. Essas features entram apenas em resposta a necessidade real, não especulativamente.

## Ephemeral Environments & Agentic Workflows

- Ambientes descartáveis são um conceito de primeira classe (`omavm run arch --ephemeral`), destruídos ao encerrar, e devem funcionar para Boxes e Machines quando possível. Casos de uso: dev temporário, testes, CI local, agentes, Agentic QA, reprodução de bugs, validação em sistema limpo.
- OmaVM deve ser controlável por humanos e por agentes: criar environments, executar comandos, consultar status, capturar logs, destruir ambientes, criar ambientes efêmeros, testar software em sistema limpo — tudo isso deve ter caminho programático (CLI/Core), nunca só via GUI.

## Open Technical Decisions

The following decisions are intentionally unresolved. Do not establish a project-wide convention for them incidentally while implementing unrelated work.

### Repository layout

The exact directory/package structure beyond the project's basic layout is not yet fixed. Prefer the smallest structure required by the current vertical slice. Do not introduce speculative layers such as `domain/`, `application/`, `infrastructure/`, `ports/`, or `adapters/` solely to conform to an architectural pattern. Refactor the layout only when concrete responsibilities justify the separation.

One concrete constraint **is** settled: no `.go` files directly in the repository root. The Core lives in `internal/core`; binaries live under `cmd/`; backends live under `internal/backend/`. This is a fixed convention, not an open question — keep new code out of the root.

### Template format

The declarative Template format is not yet defined. Examples in this document are conceptual and MUST NOT be treated as a stable schema. Do not introduce a custom OmaVM DSL or commit to TOML, YAML, JSON, or another representation until actual Template requirements are known. Prefer backend-native declarative capabilities during the initial implementation when practical.

### Integration-test organization

The strategy for tests requiring real Podman/Docker/QEMU environments is intentionally undecided. Possible approaches include:

- Go build tags;
- a dedicated integration-test directory/package;
- external test harnesses.

Do not establish one of these approaches as a repository-wide standard until real integration tests exist and their lifecycle requirements are understood. Unit/domain tests should remain runnable without requiring Podman, Docker, QEMU, KVM, or privileged host configuration.

## Rules for AI Agents

Ao trabalhar neste repositório, nunca:

1. reimplementar capacidades maduras de integração do Distrobox ou transformar a OmaVM em container runtime genérico;
2. tratar o OmaVM como se fosse apenas um frontend de QEMU;
3. acoplar a GUI diretamente a comandos shell/infraestrutura;
4. confundir Box com VM, ou aplicar a regra simplista "Linux = container, non-Linux = VM";
5. adicionar abstrações especulativas (múltiplos backends, plugin system, etc.) antes de existir um segundo caso de uso real;
6. esconder diferenças importantes de isolamento entre Box e Machine na UI, CLI ou documentação;
7. construir features grandes (Blend, guest completo, cloud, marketplace, etc.) antes de o vertical slice da Fase 1 estar funcional;
8. transformar o projeto numa coleção incoerente de wrappers em vez de um produto com modelo de domínio coerente;
9. resolver incidentalmente qualquer item listado em Open Technical Decisions (layout de repositório, formato de Template, organização de testes de integração) como efeito colateral de uma tarefa não relacionada.

Quando uma tarefa entrar em conflito com estas regras, ou com código legado existente, pare e documente o conflito em vez de resolvê-lo silenciosamente com uma mudança estrutural.
