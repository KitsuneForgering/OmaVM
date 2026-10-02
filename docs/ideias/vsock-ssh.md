# SSH e port forwarding via vsock

## O que outros fazem

O GNOME Boxes reescrito (GTK4, 2026) adiciona um dispositivo VSOCK a cada
VM. Guests com systemd v256 ou mais recente já expõem SSH nesse canal sem
configuração de rede, e o Boxes começou a oferecer port forwarding para
alcançar serviços do guest a partir do host.

## Por que interessa à OmaVM

- `omavm ssh radic` já está nos exemplos do CLI/GUI Contract do `CLAUDE.md`.
- É integração de uma ferramenta madura (systemd `systemd-ssh-generator` no
  guest, `systemd-ssh-proxy` no host: `ssh vsock/<cid>`), não um canal
  próprio. Segue a regra "integre, não reimplemente".
- Serve a agentes: `omavm exec` numa Machine hoje falha com `ErrUnsupported`
  por falta de canal no guest. O SSH por vsock pode ser esse canal para
  guests systemd, sem `omavm-guest`.

## Esboço

1. `internal/backend/machine/qemu`: `-device vhost-vsock-pci,guest-cid=<N>`, com CID
   único por Machine (≥ 3) persistido no domínio ou derivado com checagem de
   colisão. `/dev/vhost-vsock` precisa estar acessível: reportar em
   `omavm host`.
2. `omavm ssh NAME [-- args]` executa `ssh vsock/<cid>`. Falha com mensagem
   clara quando o guest não tem systemd ≥ 256 ou o `sshd` não está ativo.
3. `Exec` numa Machine: usar esse canal quando disponível, sem fingir suporte
   quando não estiver.

## Riscos e dúvidas

- Guests sem systemd (Haiku, FreeBSD, Alpine) não ganham nada; a UI não deve
  sugerir que ganham.
- Autenticação: chaves do usuário do host; nunca gerar credencial silenciosa.
- **Exposição a containers do host (oss-security, jan/2026).** O AF_VSOCK
  não é isolado por namespace de rede: qualquer processo do host, inclusive
  um container, alcança o `sshd` que o systemd ≥ 256 expõe no guest. Para a
  OmaVM isso significa que uma **Box** consegue chegar ao SSH de uma
  **Machine**, e só a autenticação separa as duas. Pelo Security Model, a
  UI não pode esconder isso. Consequências para o desenho:
  - dispositivo vsock **opt-in** por Machine, nunca adicionado a todas;
  - explicar na UI que o canal é alcançável por outros processos do host,
    incluindo Boxes;
  - no guest, a mitigação é `systemctl mask sshd-vsock.socket`, citada como
    alternativa de quem não quer o canal.
  O mesmo vale para o CID: ele é global no host, não por usuário.

## Fontes

- [The Future of GNOME Boxes – Felipe Borges](https://blogs.gnome.org/feborges/future-of-boxes/)
- [GNOME Boxes Preparing To Deliver Much Improved Virtualization Experience – Phoronix](https://www.phoronix.com/news/GNOME-Boxes-2026)
- [systemd-vmspawn(1)](https://man7.org/linux/man-pages/man1/systemd-vmspawn.1.html)
- [oss-security: Systemd vsock sshd (2026-01-02)](https://www.openwall.com/lists/oss-security/2026/01/02/1)
