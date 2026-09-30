# Ideias

Melhorias encontradas pesquisando projetos parecidos. Nada aqui é
compromisso: cada arquivo diz onde a ideia cai no escopo do `CLAUDE.md`
(Current Priorities, Scope/Non-Goals) antes de virar trabalho.

Decisão de 2026-09-28: integrações com o host entram **ligadas por padrão**
(opt-out por ambiente em Settings), nunca como opt-in. Os esboços abaixo
que dizem "opt-in" valem com essa inversão; quando há custo de segurança
(como no [vsock-ssh.md](vsock-ssh.md)), ele é dito na tela, não usado como
motivo para desligar.

| Ideia | Encaixe no escopo | Arquivo |
|---|---|---|
| SSH e port forwarding via vsock | **SSH feito 2026-09-28**, ligado por padrão com o custo dito na tela; port forwarding não | [vsock-ssh.md](vsock-ssh.md) |
| GPU por native context | Encaixa depois do Venus; hoje só AMD | [gpu-native-context.md](gpu-native-context.md) |
| Windows 11 sem ajustes manuais (TPM + Secure Boot) | Encaixa, com cuidado: a OmaVM não é "exclusiva para Windows" | [windows11-tpm-secureboot.md](windows11-tpm-secureboot.md) |
| Distrobox v2 e ações de Box (Clone, Atualizar, uso) | Risco real no export de apps; Clone já é meta do Experience Center | [boxes-distrobox.md](boxes-distrobox.md) |
| Próximos passos do display D-Bus | Encaixa: refina o viewer atual | [display-dbus.md](display-dbus.md) |
| Ambientes no launcher do Omarchy (`.desktop` por ambiente) | **Feito 2026-09-28**, opt-out | [ambientes-no-launcher.md](ambientes-no-launcher.md) |
| Terminal da Box sobre libvterm (MIT, usado pelo Qt Creator) | Encaixa, mas é decisão de arquitetura: pede acordo antes | [terminal-libvterm.md](terminal-libvterm.md) |
| Copiar do terminal da Box (seleção e OSC 52 só de escrita) | **Feito 2026-09-28**; OSC 52 opt-out | [terminal-copiar.md](terminal-copiar.md) |
| Terminal da Box à la Ptyxis (aviso ao fechar, cor do ambiente, nova sessão no mesmo diretório) | Encaixa: melhora o terminal atual sem daemon | [terminal-ptyxis.md](terminal-ptyxis.md) |
| CLI para agentes: códigos de saída por classe de erro e erros em JSON | Encaixa: agentes são meta do `CLAUDE.md`; muda contrato, documentar antes | [cli-para-agentes.md](cli-para-agentes.md) |
| Limites de CPU/memória e Travel Mode para Boxes (`podman update`) | **Travel Mode feito 2026-09-28**; limites fixos não | [box-limites-e-travel-mode.md](box-limites-e-travel-mode.md) |
| Pasta compartilhada que monta sozinha no guest (tags virtiofs no sysfs) | Encaixa, opt-in no guest; sem agente próprio | [pasta-compartilhada-automatica.md](pasta-compartilhada-automatica.md) |
| Cor do ambiente visível no Nautilus (metadados do GVFS em vez do xattr) | **Feito 2026-09-28** | [cor-no-nautilus.md](cor-no-nautilus.md) |
| Arquivar (liberar espaço) e exportar/importar ambientes | Encaixa; o formato do manifesto esbarra numa decisão em aberto | [arquivar-e-exportar.md](arquivar-e-exportar.md) |
| Progresso ao criar uma Box (pull separado, fora do lock do registro) | Encaixa: melhora UX e reduz o tempo com o registro travado | [progresso-ao-criar-box.md](progresso-ao-criar-box.md) |
| Logs no journal do systemd (protocolo nativo, sem dependência) | Encaixa: substitui a rotação própria pelo journald | [logs-no-journal.md](logs-no-journal.md) |
| Acessibilidade: leitor de tela (Orca/AT-SPI) e navegação por teclado | Encaixa: revisão com testes em `make test-qml` | [acessibilidade.md](acessibilidade.md) |
| Lista de ambientes como `QAbstractListModel` no C++ | Opcional: a correção em QML já resolve o bug; vale se crescer | [modelo-de-ambientes-em-cpp.md](modelo-de-ambientes-em-cpp.md) |
| Catálogo de sistemas para baixar | Conflita com o non-goal "dezenas de distros pré-empacotadas" | [catalogo-de-imagens.md](catalogo-de-imagens.md) |
| Espaço em disco do host; redimensionar e compactar o disco | Encaixa: pequeno, melhora a experiência | [disco-do-host.md](disco-do-host.md) |
| Machine descartável (`-snapshot` do QEMU) | Encaixa: ambientes descartáveis já são meta do `CLAUDE.md` | [machine-descartavel.md](machine-descartavel.md) |
