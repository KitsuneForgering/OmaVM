# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Este arquivo é a constituição técnica e de produto do OmaVM. Qualquer agente (humano ou IA) que trabalhe neste repositório deve segui-lo. Quando uma decisão de código conflitar com estas regras, resolva a favor destas regras ou documente o conflito explicitamente antes de prosseguir.

## Status do repositório

O repositório contém Core e CLI em Go, GUI Qt Quick/QML, backends Distrobox e QEMU/KVM, testes e CI. **A Fase 1 (vertical slice mínimo de Box e Machine) está concluída** (ver Current Priorities); a partir de 2026-09-26 o projeto está na Fase 2, com escopo liberado para os itens listados em Current Priorities → Fase 2. Recursos ainda marcados como futuros fora dessa lista continuam sendo objetivos, não capacidades entregues, e continuam gated por necessidade real (ver Coding Principles, Scope/Non-Goals). O requisito de Go é definido em `go.mod`.

Desligamento normal nunca deve virar Force Stop automaticamente. Restart usa desligamento normal seguido de Start; falhas preservam a Machine. O disco tem prioridade de boot sobre a ISO, que pode ser desconectada no próximo Start por Settings (`--disconnect-iso`). Concorrência entre processos (desde 2026-09-29): o lock do **registro** (`environments.json.lock`) só cobre ler-alterar-gravar e nunca é mantido durante uma chamada ao backend; operações que mudam **um** ambiente (start, stop, restart, pause, snapshots, remoção) seguram o lock **desse ambiente** (`locks/<id>.lock`) durante a chamada ao backend, sempre pegando-o antes do lock do registro. Criar e remover são em duas fases: o ambiente fica no registro com `operation: "creating"`/`"removing"` (status `creating`/`removing`) enquanto o backend trabalha — um pull de imagem de minutos não trava mais os outros ambientes. Um `flock` morre com o processo, então uma marca com o lock livre é uma operação interrompida: vira status `error` e o ambiente só aceita ser removido. `Open` e `Exec` não pegam lock (o `Open` de uma Box é o shell interativo). O registro é gravado como a lista simples que versões antigas leem; `Load` também lê o envelope `{"version": N, "environments": [...]}` e recusa versões mais novas — só passe a gravá-lo quando houver uma mudança incompatível de fato.

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

### Blend Mode (base em andamento na Fase 2; modo completo ainda futuro)

Meta de longo prazo: aplicações de um Environment aparecem como janelas normais do Omarchy. A partir da Fase 2 (2026-09-26), a **base** desse trabalho está em escopo — ver Current Priorities → Fase 2 — mas o modo completo (integração de bandeja, todos os tipos de aplicação, todos os backends) continua fora do escopo até a base provar valor. Não tome decisões arquiteturais na base que tornem o modo completo impossível depois.

**Lição de pesquisa (Coherence Mode do Parallels, 2026-09-26)**: usuários reclamam de indicadores de sistema duplicados (bateria, rede, relógio) quando a bandeja do guest é fundida à barra do host. Ao implementar a base de Blend Mode: indicadores de sistema do guest que o host **já mostra** não devem reaparecer — fundir aplicações, não duplicar chrome de sistema.

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
9. Snapshots são apresentados por significado ("Before system upgrade", "Clean installation"), nunca por IDs internos; detalhes de backing file/qcow2 ficam no backend. **Implementado (2026-09-27, ver Current Priorities → Fase 2)** para Machines: verbo "Go To" em vez de "revert/commit"; view dedicada (`gui/SnapshotsDialog.qml`, não misturada nas Settings); limite de histórico configurável com descarte automático do mais antigo — padrão validado externamente no Snapshot Manager do Parallels (pesquisa de 2026-09-26). Box ainda não implementa (ver Backend Rules / capabilities opcionais).
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

Implementado até agora: frontend Qt Quick/QML com toast in-app; entrada `.desktop` + ícone (`data/`) para o `omavm-gui` aparecer no launcher/taskbar com app-id próprio; sincronização direta com o tema Omarchy ativo (dark/light, background, foreground, accent e selection), acompanhando trocas em runtime. A aplicação apenas consome `colors.toml`; não mantém tema próprio. Consumir não significa repassar cegamente: texto precisa de 7:1 (WCAG 2.2 AAA 1.4.6) sobre as duas superfícies, então `Backend::loadTheme` ajusta `foreground`/`muted` (`readableTextColor`) e as cores de estado usadas como texto — verde, vermelho e `themeAccentText` — com `accessibleColor`, que mantém o tom e só clareia ou escurece o necessário; o accent cru fica para os controles do Material. `everyThemeTextMeetsAAA` confere isso em todos os temas instalados. A acessibilidade é auditada pela árvore que o Qt entrega ao AT-SPI (`tst_accessibility.qml`): controle visível sem nome ou inalcançável por teclado falha o teste. Com a acessibilidade ativa, o Qt ignora `Accessible.checked` escrito à mão e anuncia o estado do próprio controle — seleção tem que ser um controle `checkable` de verdade, e uma escolha exclusiva (as cores) leva `Accessible.role: Accessible.RadioButton`, sem o qual o Qt 6.4 não a declara marcável. Os testes consultam a `QAccessibleInterface` (`accessibilityProblems`, `accessibleState`), nunca a propriedade anexada do QML, que diverge entre versões; o CI roda o Qt 6.4 do Ubuntu 24.04 e o desenvolvimento o Qt do Arch, e os dois precisam passar.

A GUI roda ações de ambientes diferentes ao mesmo tempo (`Backend::busyEnvironments`, desde 2026-09-29): o Core já serializa cada ambiente pelo lock dele, então a GUI só recusa uma segunda ação no **mesmo** ambiente. Como OMAcut/OMAwrite, a janela principal é tiled por padrão; não adicione regra Hyprland para fazê-la flutuar. OMAcalc é uma exceção compacta explicitamente marcada como floating, não um precedente para a OmaVM.

**Quickshell (`omarchy-shell`)**: `contrib/dev.omavm.bar` é um plugin `bar-widget` real para o shell Quickshell plugin-based do Omarchy (`/usr/share/omarchy/shell`), não especulativo — mostra a contagem de Environments (via `omavm list --json`) e abre o `omavm-gui` ao clicar. `make install` já copia o plugin para `~/.config/omarchy/plugins/` (via `install-quickshell-plugin`, chamado automaticamente como dependência), mas **nunca habilitado automaticamente**: plugins de terceiros rodam sem sandbox dentro do shell já ativo do usuário, então habilitar é sempre um passo manual e explícito do usuário (`omarchy plugin enable`), nunca algo que um agente/instalador faz sozinho. Copiar o arquivo é inofensivo (só existe no disco até alguém rodar `omarchy plugin enable`); é esse comando — nunca disparado por `make install` nem por qualquer target — que carrega QML de terceiros no shell já ativo. Isso desbloqueou `omavm list`/`status --json` na CLI (já citado como aspiração no CLI/GUI Contract) — mantenha esse contrato JSON estável, é consumido por automação externa agora, não só hipoteticamente.

**Color tags + Travel Mode — implementado (2026-09-27)**, a partir da pesquisa sobre Parallels Desktop (2026-09-26): `EnvironmentSettings.Color` vem de uma paleta fixa (`core.EnvironmentColors`: red/orange/yellow/green/blue/purple/gray — não hex livre, pra não competir com o tema Omarchy) e é o único mecanismo garantido; a sincronização com o gerenciador de arquivos do host é best-effort em cima disso, nunca uma promessa (Security Model). Só a Machine (QEMU) implementa a capability opcional `core.HostLinker`: `Configure` chama `Link` quando o `--color` muda, criando/atualizando `~/OmaVM/<nome>` como symlink pro qcow2 da Machine (dá a ela uma presença real e navegável no filesystem, papel equivalente ao bundle `.pvm` do Parallels) e tentando `setfattr -n user.xdg.tags` nesse link — se `setfattr` não existir ou falhar, a cor interna continua válida mesmo assim, só a integração visual no file manager que não acontece (o Omarchy pode ou não honrar essa xattr, fora do controle da OmaVM). `Remove` chama `Unlink`. Box ainda não implementa `HostLinker` (não tem um único arquivo pra vincular) — a cor fica só interna nesse caso, sem erro. Desde 2026-09-28 o `Link` também grava `metadata::custom-icon` (GVFS, via `gio set --nofollow-symlinks`) no próprio link, apontando para o ícone da cor (`data/icons/colors/<cor>.svg`, instalado em `<data>/omavm/icons/`, achado por `desktop.ColorIcon`): é o que o **Nautilus** do Omarchy lê — ele ignora `user.xdg.tags`, que continua gravado para Dolphin/Baloo. Mesmo best-effort.

Travel Mode: `internal/backend/qemu/power.go` lê `/sys/class/power_supply` diretamente (sem depender do `upower`) só no momento do `Start`, não como daemon (`omavmd` continua fora de escopo). Se o host está na bateria, o usuário não fixou `--cpus` explicitamente e `EnvironmentSettings.TravelModeDisabled` está falso (default), o `Start` daquela sessão usa metade dos CPUs padrão (nunca sobrescrevendo o valor persistido) e loga via `slog`. Não reage a uma Machine já rodando quando a energia muda em tempo real — isso exigiria um watcher em background, que é exatamente o primeiro caso de uso concreto que justificaria um `omavmd` de verdade no futuro, mas essa decisão foi propositalmente adiada.

**Integrações são opt-out (decisão do usuário, 2026-09-28)**: toda integração nova com o host/desktop vem ligada e é desligável por ambiente em Settings — nunca um opt-in que o usuário precisa descobrir. Exceções que continuam manuais: habilitar o plugin Quickshell e a regra Hyprland pessoal (config de terceiros/pessoal que a OmaVM não escreve). Integração com custo de segurança também vem ligada quando o usuário decide assim — como o SSH via vsock —, mas o custo é **dito na tela** (Settings, `--help`, `omavm host`), nunca escondido (Security Model).

**Travel Mode para Boxes — implementado (2026-09-28)**: `internal/backend/distrobox/travel.go`, no Start/Open da Box, usa `<engine> update --cpus` (Podman, ou Docker se for ele que tem o container) por baixo do Distrobox: na bateria, metade dos CPUs; na tomada, todos de volta. Testado contra Podman 6.1 + Distrobox 1.8: o limite vale com o container parado ou rodando e sobrevive a `distrobox enter` e stop/start; `--cpus 0` e `--cpu-quota -1/0` **não** removem um limite, por isso "sem limite" é `--cpus <nproc>`. Box nunca limitada e na tomada não recebe `update` nenhum. Mesma opção opt-out `TravelModeDisabled` da Machine, agora para os dois Kinds. A leitura de energia saiu de `qemu/power.go` para `internal/power` (dois usos). Limites fixos de CPU/memória por Box continuam não implementados.

**SSH em Machines via vsock — implementado (2026-09-28), ligado por padrão por decisão do usuário**: `internal/backend/qemu/vsock.go` adiciona `-device vhost-vsock-pci,guest-cid=N` quando `/dev/vhost-vsock` é acessível e `EnvironmentSettings.SSHDisabled` é falso. O CID é aleatório acima de 65535 (longe dos sequenciais do libvirt), guardado em `<estado>/machines/<id>/vsock-cid` para a chave do host não mudar; se o QEMU recusar (`unable to set guest cid: Address already in use`, outro VM tem o CID), sorteia outro e tenta uma vez mais. O CID em uso é lido do `/proc/<pid>/cmdline` do QEMU. `omavm ssh NOME [--user U] [-- CMD]` (capability `core.RemoteShell`) e `omavm exec` numa Machine (antes `ErrUnsupported`; agora `BatchMode=yes`, argumentos citados para o shell remoto) rodam `ssh vsock/<cid>` com `systemd-ssh-proxy`; as opções na linha de comando vencem o `ssh_config.d` do systemd, que loga como root e desliga a checagem de chave: aqui o login é o usuário do host e a chave fica em `known_hosts` sob `HostKeyAlias=omavm-<id>` (`accept-new`). O guest precisa de systemd ≥ 256 com sshd; sem isso, erro claro. **Custo de segurança dito na tela**: o CID é global no host, então qualquer processo, Boxes incluídas, alcança o sshd do guest e só o login dele protege — Settings mostra isso em texto (não só tooltip), assim como `--help` e `omavm host`. Validado com QEMU real: sobe sem root, `savevm`/`loadvm` funcionam com o dispositivo, e `omavm ssh` chega ao CID pelo proxy (sem sshd no guest de teste, cai na mensagem de ajuda); login num guest real **não** foi verificado.

**Entrada no launcher por ambiente — implementado (2026-09-28), opt-out**: `internal/desktop` escreve `$XDG_DATA_HOME/applications/dev.omavm.env.<id>.desktop` para cada ambiente (nome pelo ID, que nunca muda). `Exec` de Machine é `omavm open NOME` (inicia se preciso e abre o viewer); de Box é `omavm-gui --terminal NOME` (ou `omavm open` com `Terminal=true` sem a GUI). Ícone pela cor do ambiente. A Core só conhece a interface `core.Launcher` (`Service.SetLauncher`, injetada pelo `cmd/omavm`): publica no Create/Configure e também no Start/Open (o que cobre ambientes criados antes), retira no Remove e quando `EnvironmentSettings.LauncherDisabled` (`--launcher=false`, Settings → Automation). Falha de escrita nunca falha a operação. `make uninstall` apaga as entradas (apontariam para binários que sumiram); `omavm` as recria no próximo start.

**Ajuste de UX (2026-09-27)**: Travel Mode e o clipboard-share (abaixo) são **opt-out, não opt-in** — ligados por padrão, persistidos por Machine (`omavm settings NOME --travel-mode=false`/`--share-clipboard=false`, ou Settings → Automation na GUI), nunca um checkbox de sessão que o usuário precisa lembrar de reativar toda vez que abre o viewer. `EnvironmentSettings.ClipboardDisabled`/`TravelModeDisabled` guardam o estado invertido de propósito: o zero-value do Go (`false`) já significa "habilitado", então um `environments.json` antigo sem esses campos continua com tudo ligado.

**Logs**: ambos os binários usam `log/slog` (JSON). `/var/log` é root-owned por padrão; a OmaVM nunca eleva privilégio silenciosamente para escrever lá (ver Security Model). O destino é `/var/log/omavm/<component>.log` apenas se esse diretório já existir e for gravável; caso contrário cai para `$XDG_STATE_HOME/omavm/logs/`. Ver README seção "Logs" para o passo opcional (`sudo install -d ...`) que provisiona `/var/log/omavm`. Desde 2026-09-28: rotação por tamanho (acima de 5 MiB o log vira `<component>.log.1`), "log opened" só na primeira linha de cada arquivo, e comandos somente leitura (`list`, `status`, …, que a GUI e a barra rodam a cada poucos segundos) em nível Debug, que não é gravado — o log de 1,5 MB em dois dias era quase todo poll.

**OmaStore (2026-09-29)**: `omastore.toml` na raiz lista a OmaVM na loja; valide com `omastore lint-manifest .` (estrito). O manifesto declara o que as heurísticas errariam: o executável é `bin/omavm-gui` (pelo nome do repositório a loja escolheria a CLI) e o ícone `data/icons/dev.omavm.app.svg`. O asset é o tarball plano de `make dist` (`bin/omavm`, `bin/omavm-gui`, `data/`), publicado por `.github/workflows/release.yml` a cada tag `v*`, compilado em `archlinux:latest` porque a GUI linka o Qt 6 do sistema. Instalado fora do `/usr`, cada binário acha o outro por ser vizinho em `bin/`, e `internal/desktop` usa os ícones de `data/` ao lado (`ColorIcon`, `appIcon`) em vez de nomes de tema que ninguém instalou.

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
- O JSON da CLI (`list`, `list --status`, `status`, `settings`, `snapshot list`) é contrato com GUI, barra e agentes: `TestJSONContract` compara com `cmd/omavm/testdata/*.golden.json`. Uma mudança intencional é regravada com `go test ./cmd/omavm -run TestJSONContract -update` e revisada no diff.
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

Features explicitamente fora do escopo até que uma necessidade real exista: Blend Mode completo (a base está em construção na Fase 2, ver Current Priorities), guest agent completo, GPU passthrough, orchestration distribuída, cloud, remote hosts, marketplace, dezenas de distros pré-empacotadas, networking editor avançado, plugin system.

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

### Fase 1 — vertical slice mínimo (concluída em 2026-09-26)

**Development Box (via Distrobox sobre Podman/Docker):** Create, Start/Open, Stop, Status, Exec, Remove. O adapter direto anterior permanece apenas para lifecycle de Environments cujo campo `backend` já seja `podman` ou `docker`.

**Machine (via QEMU/KVM):** Create, Start, Stop, Status, Open, Remove.

**Implementado (2026-09-26, display migrado para D-Bus em 2026-09-28)**: `Open` numa Machine abre de fato o userspace gráfico dentro do próprio `omavm-gui`. A Machine roda headless com o **D-Bus display do QEMU em modo peer-to-peer** (`-display dbus,p2p=yes,gl=on` + `virtio-vga-gl`; sem render node utilizável no host, `virtio-vga` + `dbus,p2p=yes`). Ninguém escuta até o `Open`: `qemu.Backend.attachDisplay` cria um socketpair, entrega uma ponta ao QEMU via QMP (`getfd` com SCM_RIGHTS + `add_client protocol=@dbus-display`) e a outra ao viewer como fd herdado (`omavm-gui --display-fd 3 --title <nome> --share-clipboard=...`). Detalhes de QMP ficam no adapter; o viewer só fala o protocolo de display. Machines iniciadas antes dessa migração (ainda com VNC) recebem "restart it" em vez de um erro de QMP (resposta real do QEMU 11.1: `DeviceNotActive: D-Bus display is not in use`).

Substituiu o cliente RFB próprio (removido): com `gl=on` os quadros chegam como **dma-bufs** (`ScanoutDMABUF`/`UpdateDMABUF`) e são importados como textura GL via EGL (`EGL_EXT_image_dma_buf_import`, com modifier — a Intel manda tiling X) sem cópia pela CPU; o QEMU segura a GPU do guest até cada `UpdateDMABUF` ser respondido, e `DisplayClient` responde depois do `frameSwapped` (ou em 50 ms, para janela oculta nunca travar o guest). Sem GPU, `ScanoutMap`/`UpdateMap` compartilham a superfície por memfd. `gui/displayclient.h/.cpp` (protocolo, sem GUI) usa **GDBus (gio, LGPL, link dinâmico)** e não QtDBus: toda conexão aqui é um socket já conectado entregue por outro processo, e o QtDBus não adota fd. O QEMU é servidor de autenticação nas duas conexões (principal e listener). `gui/displayview.h/.cpp` (`QQuickItem`) desenha e encaminha entrada; o viewer força o RHI OpenGL (`QQuickWindow::setGraphicsApi`). Orientação: o `y0_top` do QEMU é relativo ao framebuffer GL de baixo para cima (a UI GTK dele inverte quando é falso); o scene graph amostra de cima para baixo, então o viewer inverte quando `y0_top` é **verdadeiro** — validado por teste.

Entrada: teclado por scancode físico (`gui/keymap.h`, evdev → qnum do QEMU), então o layout do guest manda (dead keys, AltGr); auto-repeat encaminha só os press; teclas pressionadas são soltas ao perder o foco. Ponteiro absoluto via `usb-tablet` (`qemu-xhci`); sem `IsAbsolute`, cai para movimento relativo. O cursor do guest (`CursorDefine`/`MouseSet`) vira o cursor do host — até o guest definir um, o host esconde o seu (o guest pode desenhar o próprio). Redimensionar a janela envia `SetUIInfo` (debounce de 250 ms, tamanho físico em mm para o DPI); só a virtio-gpu honra, a VGA padrão responde "unsupported".

Clipboard de texto UTF-8 (até ~1 MiB) usa a interface `org.qemu.Display1.Clipboard` nos dois lados, com o guest ligado pelo canal `qemu-vdagent`/`com.redhat.spice.0`; não compartilha imagens ou arquivos. O QEMU chama o `Request` do viewer de forma **síncrona** (trava o loop dele), então a resposta é imediata; só um peer de clipboard por Machine (um segundo viewer segue sem). Se está ativo é uma configuração da Machine (`EnvironmentSettings.ClipboardDisabled`, opt-out, ligado por padrão), repassada como `--share-clipboard`; só vale com a janela ativa. O guest precisa de `spice-vdagent` funcional na sessão gráfica; QGA conectado não comprova essa integração. Testes: `make test-display` sobe um QEMU real sem disco — `displayclient_test` cobre memória compartilhada, dma-buf e o pacing; `displayview_test` renderiza fora da tela (`QQuickRenderControl`, nenhuma janela aparece) e confere a orientação nos dois caminhos; pula sem KVM/Wayland. Áudio (`virtio-sound-pci`/pipewire), virtiofs e QGA são independentes do display.

O posicionamento **estático** da janela do viewer (fullscreen, workspace dedicado) é feito por uma regra Hyprland opt-in (`contrib/hypr/omavm-viewer.lua`, `o.window("dev.omavm.viewer", { workspace = "name:omavm", fullscreen = true })`), avaliada em tempo de config, não em runtime. Como o plugin Quickshell, essa regra não é injetada automaticamente no config do usuário — é adicionada manualmente ao `windows.lua` pessoal (documentado no README), no mesmo padrão de regras pessoais já usado lá (`o.window(...)` direto, sem módulo separado).

**Runtime dispatch validado (2026-09-27)**: esta build de Hyprland do Omarchy é um fork com config em Lua, e `hyprctl dispatch <ARGS>` nela avalia `<ARGS>` como código Lua (`hl.dispatch(<ARGS>)`, literal, não tokenizado por shell) — por isso um dispatch clássico do Hyprland puro com seletor via string (`hyprctl dispatch focuswindow "class:^(...)$"`) falha aqui ("')' expected"): esses tokens brutos não formam Lua válido. A forma correta, confirmada contra uma sessão real e igual ao padrão que o próprio Quickshell do Omarchy usa (`/usr/share/omarchy/shell/plugins/bar/widgets/Workspaces.qml`), é um único argumento com uma expressão Lua válida usando a API estruturada `hl` (documentada em `/usr/share/hypr/stubs/hl.meta.lua`, leitura sempre segura): `hyprctl dispatch 'hl.dsp.focus({ workspace = "3" })'`. Consultas (sem side effect) usam `hyprctl repl '<código lua>'`, que imprime o valor de retorno — ex.: `hl.get_workspaces()` traz `HL.Workspace[]` com `.id`/`.name`/`.is_empty`/`.special`/`.monitor`/`.visible` já prontos (sem precisar inferir "vazio" por parsing), e `hl.get_windows({ class = ..., title = ... })` filtra por igualdade exata, sem regex. `$HYPRLAND_INSTANCE_SIGNATURE` (env var, sem spawn de processo) é o jeito barato de checar se há sessão Hyprland antes de chamar `hyprctl`. Ficou confirmado que `hl.dsp.focus({ window = <objeto> })` (focar uma janela específica não-ativa, em vez do workspace inteiro) não muda o foco de forma confiável nesta sessão de teste (focus-follows-mouse provavelmente reafirma o foco); só `hl.dsp.focus({ workspace = ... })` foi validado como confiável — ver `gui/backend.cpp`'s `placeInEmptyWorkspace` para o uso real. Essa descoberta habilitou a preferência opt-in "Open in an empty workspace" (`EnvironmentSettings.OpenInEmptyWorkspace`, ver Current Priorities → Fase 2), mas continua não substituindo a regra estática acima para fullscreen/workspace dedicado — as duas soluções coexistem para necessidades diferentes, ver nota de conflito logo abaixo.

Duas pegadinhas resolvidas nessa regra (2026-09-26): (1) workspace **especial** (`special:...`) nunca fica visível automaticamente — é um overlay estilo scratchpad que só aparece com `togglespecialworkspace` explícito; por isso a regra usa uma workspace **nomeada** normal (`name:omavm`), que troca a view do monitor sozinha quando a janela abre. (2) `gui/Viewer.qml` não pede fullscreen pelo próprio QML (`visibility: Window.FullScreen`) — pedir dos dois lados (client Wayland e regra do compositor) vira um toggle e cancela um ao outro, deixando a janela destilada em vez de fullscreen. Desde 2026-09-29 (pedido do usuário) o viewer de Machine abre em tela cheia **por padrão** (`EnvironmentSettings.FullscreenDisabled`, opt-out, só Machine; `--fullscreen=false`, Settings → Automation): `gui/main.cpp` só pede tela cheia (`showFullScreen()` no `Viewer.qml`) quando `hasPersonalViewerRule` não acha regra pessoal — com a regra, ela manda e o pedido duplo continua evitado. O terminal de Box não vai para tela cheia: sozinho num workspace vazio ele já ocupa a tela.

**Implementado (2026-09-27)**: o "Open" de uma Development Box também abre dentro do `omavm-gui`, em vez de um terminal externo (`xdg-terminal-exec`, removido). `gui/backend.cpp` relança o próprio binário em modo `--terminal <nome> --title <título>` (mesmo padrão do `--display-fd` de Machine), que carrega `gui/TerminalViewer.qml` reaproveitando de propósito o mesmo app id `dev.omavm.viewer` — a regra Hyprland opt-in acima cobre os dois sem configuração adicional. `internal/backend/distrobox`'s `Open()` (anexação interativa via `distrobox enter`, stdio herdado) não muda: o modo `--terminal` só cria um PTY próprio (`forkpty`, glibc) e executa `omavm open <nome>` dentro dele — o mesmo comando que já funcionava num terminal real — então `omavm open` continua funcionando normalmente para humanos, scripts e agentes fora da GUI.

O emulador (`gui/terminal.h/.cpp`, `gui/terminalview.h/.cpp`) é implementado do zero — mesma razão de licença do antigo cliente RFB (Backend Rules/Coding Principles: projeto MIT, viewers de terminal maduros como QTermWidget são GPL). Parser Ground/Escape/CSI/OSC cobre: movimento e endereçamento de cursor, SGR (16 cores, 256 cores, truecolor `38/48;2`), scrolling region, alternate screen buffer (`?1049`/`?47` — obrigatório pra vim/htop/less/tmux não corromperem a tela primária), scrollback limitado, título via OSC 0/2, e largura de caractere (desde 2026-09-28): CJK e emoji ocupam 2 colunas (célula-cabeça + célula de continuação com `ch` vazio), acentos combinantes se juntam à célula anterior, e escrever sobre metade de um caractere largo apaga a outra metade. Teclado (`TerminalSession::keySequence`, testável sem janela): segue o que `TERM=xterm-256color` promete — modo de aplicação das setas (DECCKM `?1h`, que o ncurses liga via `smkx` e depois só reconhece `\EOA`), modificadores no formato do xterm (`\E[1;5D` para Ctrl+←), `\E[Z` para Shift+Tab e os controles Ctrl+Espaço/`[`/`\`/`]`/`^`/`_`. A largura vem das tabelas Unicode do Qt, não do `wcwidth()` da glibc, que depende do locale (devolve -1 para tudo fora do ASCII com `LANG=C`, como no CI). Copiar (desde 2026-09-28): arrastar seleciona (vira seleção primária; clique do meio cola), clique duplo seleciona palavra/caminho, Ctrl+Shift+C e Ctrl+Insert (o Super+C do Omarchy) copiam — nunca chegam ao programa como Ctrl+C; a seleção não corta caractere largo e sobrevive ao descarte do scrollback (`droppedLines`). OSC 52 só de **escrita** (Neovim/tmux copiam para o host); pedido de leitura (`?`) é ignorado, porque deixaria qualquer coisa na Box ler o clipboard do host. É a mesma configuração opt-out `ClipboardDisabled` da Machine, que agora vale para os dois Kinds (repassada como `--share-clipboard`). O buffer de OSC é limitado (`kMaxOscLength`): um OSC sem terminador antes crescia sem limite. Parâmetros CSI são limitados a 65535, como no xterm (2026-09-29): achado por fuzz com ASan/UBSan, `ESC[2147483647C` com o cursor fora da coluna 0 estourava o int, levava o cursor a uma coluna negativa e o próximo erase escrevia fora da linha — segfault disparável por qualquer programa na Box ou por um `cat` de arquivo. `randomInputKeepsTheScreenConsistent` é um fuzz curto de semente fixa na suíte normal. Fora de escopo por enquanto (falha graciosamente, não quebra): mouse reporting, hyperlinks OSC 8, sixel. A paleta ANSI vem de `colors.toml` (via `gui/colorstoml.h`, extraído de `Backend::loadTheme()` — mesmo parser, dois usos reais agora) em vez de cores fixas, mesmo raciocínio de sync com o tema Omarchy do resto da GUI. Testes em `gui/tests/terminal_test.cpp` (`make test-terminal`) cobrem o parser via `feed()` direto (sem PTY) e dois testes de PTY real (`forkpty`+`execvp`+leitura+escrita via `/bin/sh`/`/bin/cat`).

Existe também `Previewer` (`internal/core/backend.go`), uma capability **opcional** do `Backend` (não faz parte da interface mínima acima): a Machine implementa via QMP `screendump` (independente do protocolo de display); Box não implementa — não tem display, e isso não deve ser escondido atrás de um placeholder que finja equivalência (Security Model). A GUI é o frontend Qt Quick/QML em `gui/` (`gui/EnvironmentCard.qml` mostra a miniatura por card só quando o Backend a oferece) — o antigo binding Go em `internal/gui/` foi removido.

### Fase 2 — escopo liberado em 2026-09-26

Com a Fase 1 pronta, os três itens abaixo (pesquisa Parallels de 2026-09-26, ver Omarchy Integration e UX Principles #9) saem de "ideia validada" para prioridade ativa de implementação. Seguir o Development Workflow (domínio → backend → Core → CLI → GUI → testes → docs) para cada um; não pular para a GUI.

**Snapshot Manager — implementado (2026-09-27)**: `core.Snapshot{ID, Label, CreatedAt}` vive em `Environment.Snapshots`, persistido no mesmo `environments.json` (sem storage novo). `ID` é o tag técnico que o Core gera (`label` sanitizado + sufixo aleatório curto); `Label` é o único texto que CLI/GUI mostram (UX Principle #9). `EnvironmentSettings.SnapshotLimit` (default 10) descarta o mais antigo automaticamente a cada `CreateSnapshot` acima do limite — tanto no histórico do Core quanto no backend (`Service.CreateSnapshot` chama `RemoveSnapshot` do mais antigo antes de persistir).

Disco da Machine (desde 2026-09-29, pedido do usuário): qcow2 esparso de **1 TiB** virtual (`qemu.diskSize`), que começa com ~200 KiB e só cresce conforme o guest escreve. `Start` amplia para 1 TiB (`growDisk`, `qemu-img resize`, nunca encolhe, best-effort) discos menores — Machines antigas de 20 GiB e qualquer uma que voltou a uma snapshot anterior, porque a snapshot do qcow2 guarda o próprio tamanho (verificado no QEMU 11.1). Se o disco do host enche, o QEMU pausa a Machine (`io-error`), que aparece como `paused` com o motivo e sai com Resume.

O estado de cada Machine fica em `<estado>/machines/<id>/` (desde 2026-09-29; antes era `<nome>`): o nome nunca vira componente de caminho e não pesa no limite de 108 bytes dos sockets. `qemu.Backend.key` move o diretório antigo para o ID na primeira vez que vê a Machine parada (um QEMU rodando só é reconhecido pelo caminho do pidfile que recebeu) e refaz o link `~/OmaVM/<nome>`.

Só a Machine (QEMU) implementa a capability opcional `core.SnapshotManager`; Box retorna `ErrUnsupported` (mecanismo nativo do Distrobox/Podman para isso ainda não decidido — não implementado especulativamente). O adapter QEMU usa snapshots internos do qcow2, **só de disco**: parada, `qemu-img snapshot -c/-a/-d` direto no arquivo; rodando, `blockdev-snapshot-internal-sync`/`blockdev-snapshot-delete-internal-sync` via QMP no disco `virtio0`. Com o QGA respondendo (desde 2026-09-29), o snapshot é cercado por `guest-fsfreeze-freeze`/`guest-fsfreeze-thaw` (o thaw sempre roda, com uma nova tentativa em outra conexão); sem QGA, ou se o guest recusar o freeze, o snapshot segue como se a energia tivesse sido cortada naquele instante, e fica marcado `crash_consistent` — o SnapshotsDialog diz isso ao lado da data. Testado só contra um agente falso (`TestSnapshotOfRunningMachineFreezesTheGuest`, `TestGuestSyncSkipsStaleReplies`); o freeze num guest Linux real com `qemu-guest-agent` **não** foi verificado. Até 2026-09-29 a Machine rodando usava `savevm`/`loadvm` (HMP), que **nunca funcionou na configuração real**: testado no QEMU 11.1, todo Machine tem `virtio-sound` ("State blocked by non-migratable device") e, com 3D, `virtio-vga-gl` ("virgl is not yet migratable"). `GoToSnapshot` exige a Machine parada — o QEMU só reverte snapshot de disco offline ("Revert to it offline using qemu-img") —, e o SnapshotsDialog desabilita "Go To" e explica isso enquanto ela roda. Regressões: `TestSnapshotOfRunningMachineIsDiskOnly`, `TestGoToSnapshotNeedsAStoppedMachine`, `tst_snapshotsdialog.qml`.

CLI: `omavm snapshot create <nome> --label "texto"` (flag pode vir antes ou depois do nome, como os demais comandos — `extractValueFlag` resolve isso do mesmo jeito que `extractBoolFlag` já fazia pro `--json`), `snapshot list [--json]`, `snapshot go-to <nome> <id>`, `snapshot remove <nome> <id>`; `settings <nome> --snapshot-limit N` ajusta o limite. GUI: `gui/SnapshotsDialog.qml` (aberto via "Snapshots…" no menu do card, só para Machines) lê `environment.snapshots` direto do JSON que `list --json` já traz — sem chamada extra de enriquecimento — e expõe criar/"Go To"/apagar.

**Color tags + Travel Mode**: tag de cor por Environment persistida no domínio, refletida no Experience Center e, quando o gerenciador de arquivos do Omarchy suportar, sincronizada com ele. Travel Mode: detectar Machine rodando com o host na bateria (via mecanismo já exposto pelo sistema, não reimplementar detecção de energia) e reduzir rede/recursos automaticamente — automação de host, não um toggle manual escondido em Settings.

**Blend Mode (base) — implementado (2026-09-27)**: para Boxes, expõe `distrobox-export` (recurso nativo do Distrobox — Backend Rules: integrar, não reimplementar) via `core.AppExporter` (`ListApps`/`ExportApp`/`UnexportApp`). Verificado contra um container Distrobox real: `distrobox-export --app` casa por substring (case-sensitive) em `Exec=`/`Name=` do `.desktop`, o que é ambíguo (nomes com espaço, maiúsculas, regex) — por isso o `App.ID` que a OmaVM usa é o **caminho absoluto do arquivo `.desktop`** dentro da Box (`--app /caminho/completo.desktop` funciona tanto pra exportar quanto pra `--delete`, confirmado). `ListApps` só varre `/usr/local/share/applications` e `/usr/share/applications` dentro da Box — nunca `~/.local/share/applications`, que é o **mesmo diretório do host** (Distrobox compartilha `$HOME`): qualquer app já ali já aparece no host sem exportar nada. Detectar se um app já foi exportado não precisa entrar na Box: o arquivo exportado sempre aparece em `~/.local/share/applications/<nome-do-container>-<basename>.desktop`, então `os.Stat` direto no host resolve. Machine não implementa (integração de janela completa é o "modo completo", gated).

CLI: `omavm apps <nome> [--json]`, `omavm apps <nome> --export <app-id>`, `omavm apps <nome> --unexport <app-id>`. GUI: `gui/AppsDialog.qml` (menu "Applications…" no card, só para Boxes) chama `backend.refreshApps` ao abrir e `exportApp`/`unexportApp` por item — diferente de Snapshots, a lista de apps não vem embutida no `list --json` do Environment (mudaria a cada chamada e não é um dado que precisa persistir no domínio), então tem sua própria chamada `apps --json` sob demanda.

**"Open in an empty workspace" — implementado (2026-09-27), item de P2 (`docs/TODO.md`), não um dos três originais da Fase 2**: adicionado ao escopo por pedido direto do usuário (necessidade real documentada, não decisão especulativa do agente — Rules for AI Agents #7), depois de validar a API `hl` acima. **Ligado por padrão desde 2026-09-28** (decisão do usuário, ver "Integrações são opt-out"): `EnvironmentSettings.EmptyWorkspaceDisabled` guarda o opt-out; a chave antiga `open_in_empty_workspace` (opt-in) é simplesmente ignorada, porque quem a ligou queria o que agora é o padrão. Aplica-se aos dois Kinds. Desde 2026-09-29 quem posiciona é o **próprio viewer**, na inicialização dos modos `--display-fd`/`--terminal` em `gui/main.cpp` (`placeViewer`, flag `--empty-workspace`), antes de a janela existir: antes vivia no `Backend::open()` da GUI, e por isso uma Machine aberta pela entrada do launcher ou por `omavm open` (que lança o viewer direto de `qemu.Backend.Open`) nunca ia para um workspace vazio. O Core/Go só repassa a configuração (`qemu.viewerArgs`, entrada do launcher da Box); a política continua na integração com o desktop. Fluxo: se a preferência está ligada, `placeInEmptyWorkspace(title)` primeiro procura uma janela já aberta para esse ambiente (`hl.get_windows({class="dev.omavm.viewer", title="<nome> — OmaVM"})`, mesmo título que `qemu.Backend.Open`/`gui/backend.cpp`'s modo `--terminal` já usam) e, se achar, só troca pro workspace dela (não abre outra cópia); senão escolhe o primeiro workspace elegível no monitor ativo (vazio via `HL.Workspace.is_empty`, não especial, não pertencente a outro monitor, incluindo um número ainda não materializado) e troca a view pra lá antes de criar a janela — ela abre onde o Hyprland já está focado. Se já havia uma janela desse ambiente, o novo viewer foca o workspace dela e sai sem abrir outra.

Lacunas conhecidas, deixadas como fallback honesto em vez de resolvidas por adivinhação: (1) focar a janela específica (não só o workspace dela) não ficou confiável nos testes (ver nota de `hl.dsp.focus({window=...})` acima) — só o workspace é focado; (2) não há API pra distinguir um workspace "reservado por regra pessoal" de um workspace comum não-vazio, então regras pessoais não são detectadas, só workspaces vazios são pulados por estarem ocupados; (3) a busca é limitada a uma faixa fixa (1–30), não a uma "faixa configurada pelo usuário" que não existe como conceito consultável no Hyprland. Nenhuma dessas lacunas finge suporte que não existe — `WorkspacePlacement::Unavailable` cai no comportamento anterior (abrir no workspace atual) com uma mensagem. Sem sessão Hyprland (`NotApplicable`) a queda é silenciosa: com o padrão ligado, avisar a cada Open seria ruído.

**Conflito conhecido com a regra estática do viewer**: se o usuário já carregou `contrib/hypr/omavm-viewer.lua` (a regra opt-in acima, que fixa `dev.omavm.viewer` sempre no workspace nomeado `name:omavm` em fullscreen), essa regra de janela tem precedência sobre "qual workspace estava focado quando a janela abriu". Desde 2026-09-28 (padrão ligado), `hasPersonalViewerRule()` em `gui/backend.cpp` procura `dev.omavm.viewer` fora de comentários em `~/.config/hypr/**/*.lua|*.conf`; se achar, a OmaVM não troca de workspace (a regra pessoal manda) — senão o usuário ficaria olhando um workspace vazio enquanto a regra move a janela. A config pessoal nunca é reescrita.

**Diagnóstico do host e Vulkan — implementado (2026-09-28), por pedido direto do usuário**: `omavm host [--json]` (capability opcional `core.HostInspector`, só a Machine implementa, em `internal/backend/qemu/hostcaps.go`) informa KVM, OpenGL (render node + `virtio-vga-gl`), Vulkan e se dá para dedicar uma GPU a uma Machine. Só lê o host (sysfs, `/dev`, ICDs do Vulkan): nunca carrega módulo, liga driver ou muda permissão. O item de passthrough é **diagnóstico apenas** — GPU passthrough/VFIO e Looking Glass continuam fora do escopo abaixo (exigem configuração privilegiada do host, inviável com uma GPU só, e o Looking Glass mira guests Windows). Vulkan no guest (Venus: `virtio-vga-gl,blob=on,hostmem=4G,venus=on` + memória memfd) é **ligado por padrão quando a detecção passa** — só Mesa Intel/AMD, ICD instalado com biblioteca presente, `virgl_render_server`, `/dev/udmabuf` gravável — com opt-out por Machine (`EnvironmentSettings.VulkanDisabled`, `--vulkan=false`, Settings → Graphics). A falha do Venus não é detectável no Start (o virglrenderer só inicializa quando o driver do guest fala com a GPU), por isso a detecção é estática. Validado no host (QEMU sobe, Haiku renderiza com blob ligado); a aceleração dentro de um guest Linux **não** foi verificada — o usuário preferiu não baixar uma ISO de teste.

Continua fora do escopo até necessidade real: guest agent completo, GPU passthrough, orchestration distribuída, cloud/remote hosts, marketplace, dezenas de distros, networking editor avançado, plugin system, e o Blend Mode completo (além da base acima).

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
7. construir features fora da lista de Current Priorities → Fase 2 (guest completo, cloud, marketplace, GPU passthrough, orchestration, networking editor avançado, plugin system, Blend Mode completo além da base) sem que exista necessidade real documentada — a Fase 1 está concluída, mas isso libera especificamente os três itens da Fase 2, não escopo ilimitado;
8. transformar o projeto numa coleção incoerente de wrappers em vez de um produto com modelo de domínio coerente;
9. resolver incidentalmente qualquer item listado em Open Technical Decisions (layout de repositório, formato de Template, organização de testes de integração) como efeito colateral de uma tarefa não relacionada.

Quando uma tarefa entrar em conflito com estas regras, ou com código legado existente, pare e documente o conflito em vez de resolvê-lo silenciosamente com uma mudança estrutural.
