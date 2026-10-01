# Propostas para descoberta

Catálogo separado do trabalho ativo em [TODO.md](TODO.md). Preserva as propostas e critérios registrados em 2026-09-27; não autoriza implementação nem amplia o escopo de [CLAUDE.md](../CLAUDE.md).

## P3 — Descoberta antes de ampliar o escopo

Estes itens são hipóteses de evolução. Cada um exige um caso de uso demonstrado, análise das capacidades existentes e decisão registrada antes de virar implementação.

- [ ] **Instalação mais assistida:** avaliar catálogo pequeno de imagens e download verificável, com origem, tamanho, licença e recuperação de falhas; manter ISO local como caminho completo.
- [ ] **Blend Mode para Machines:** demonstrar um fluxo real de aplicação integrada antes de definir infraestrutura. A base de exportação das Boxes não comprova viabilidade para Machines; evitar duplicar indicadores de sistema do host.
- [ ] **Transferência de arquivos e arrastar/soltar:** escolher uma combinação host/guest suportada, testar permissões e falhas e preferir capacidades existentes a um protocolo próprio.
- [ ] **Clone e recuperação mais guiada:** clone completo feito em 2026-10-01 (ver a triagem em `TODO.md`); falta o vinculado e a recuperação guiada. validar demanda e capacidades do backend; explicar consumo de disco e dependências antes de oferecer modalidades de clone.
- [ ] **Organização de grandes listas:** validar necessidade de favoritos, grupos e reordenação; quando implementados, persistir o comportamento de verdade.
- [ ] **Integrações adicionais:** avaliar áudio, dispositivos e múltiplos monitores por jornada concreta, deixando explícita a matriz de suporte. Não assumir paridade com o Parallels por adotar sua referência de UX.

### Catálogo de capacidades de virtualização a avaliar

Este catálogo incorpora a lista de evolução proposta pelo usuário. Antes de implementar, inventariar o que já está configurado no backend, o que está exposto no Core/CLI e o que foi validado em guest real. “Não exposto na GUI” não significa “ausente no QEMU”. Os critérios abaixo orientam a descoberta e a futura entrega; não prometem todos os recursos nem alteram automaticamente o escopo de `CLAUDE.md`.

Cada proposta deve registrar caso de uso, suporte do backend/guest, necessidade de privilégio, comportamento com a Machine ligada/desligada, recuperação de falhas e decisão de escopo. Começar com operações offline quando suficientes; hotplug exige necessidade demonstrada. Networking avançado e guest agent completo continuam exigindo revisão explícita das diretrizes antes de implementação.

#### Storage e portabilidade de imagens

- [ ] **Discos adicionais:** avaliar criar, anexar, listar e desanexar discos com identidade estável, distinguindo disco de sistema e dados. Aceite: desanexar não apaga o arquivo; exclusão é explícita; uma imagem em uso não é anexada para escrita concorrente indevida.
- [ ] **Resize de disco:** começar pelo crescimento do disco virtual, com estado permitido e limites claros. Aceite: diferenciar capacidade do disco de partição/filesystem no guest e orientar a expansão interna; redução de tamanho fica fora do fluxo inicial até haver procedimento seguro validado.
- [ ] **Thin provisioning e descarte:** inventariar formato e alocação usados; avaliar indicação de capacidade virtual versus espaço realmente ocupado e recuperação por discard/TRIM. Aceite: medir antes/depois em guest suportado e explicar risco de o host ficar sem espaço; não apresentar capacidade virtual como espaço reservado.
- [ ] **Backing chains:** descobrir e validar dependências antes de mover, excluir ou exportar imagens. Aceite: operação não quebra um descendente; base ausente ou alterada gera diagnóstico; alterações de cadeia exigem plano de recuperação e estado de execução compatível.
- [ ] **Snapshots internos versus externos:** documentar a estratégia atual e avaliar alternativas por restauração, portabilidade e custo. Aceite: distinguir snapshot de disco de captura de memória, consistência com Machine ligada/desligada e relação com backing chains; evitar expor duas modalidades sem caso de uso. Complementa a auditoria e as confirmações de snapshots, sem duplicá-las.
- [ ] **Import/export de imagens:** definir formatos e configuração mínima suportados, incluindo discos existentes além de ISOs de instalação. Aceite: origem preservada, conversão com espaço estimado e falha recuperável; exportação identifica dependências ou gera imagem independente; importar o resultado em um novo ambiente comprova o fluxo.

#### Networking

- [ ] **NAT simples:** inventariar o modo atual e validar saída de rede e resolução de nomes em guests suportados. Aceite: a experiência básica funciona sem configuração manual de bridge e a UI explica o alcance da conectividade.
- [ ] **Port forwarding:** avaliar regras por Machine com protocolo, porta do host, destino e endereço de escuta explícitos. Aceite: detectar conflito de portas, remover regras corretamente e usar loopback como padrão para serviços locais; exposição à LAN exige escolha visível.
- [ ] **Bridge:** demonstrar um caso de acesso direto pela LAN, descobrir bridges existentes e documentar restrições do host. Aceite: permissões e mudanças são explícitas e reversíveis; não alterar a rede ativa do host silenciosamente.
- [ ] **Redes privadas entre Machines:** definir isolamento, endereçamento e eventual saída para a internet. Aceite: duas Machines na mesma rede se comunicam e uma fora dela não recebe conectividade indevida; criação/remoção não deixa recursos órfãos.
- [ ] **Perfis de rede:** avaliar apenas após os modos anteriores terem uso demonstrado. Preferir intenções como “internet”, “somente entre ambientes” e “rede local”; evitar um editor genérico de infraestrutura. Aceite: perfil descreve alcance e exposição reais, persiste e informa quando exige reinício.

#### Hardware virtual

- [ ] **CPU topology e memória:** inventariar controles atuais de CPU/RAM antes de ampliar opções; avaliar sockets/cores/threads e modelo de CPU quando houver necessidade de compatibilidade ou desempenho. Aceite: recursos efetivos correspondem ao configurado, limites são validados e mudanças pendentes de reinício aparecem claramente.
- [ ] **Virtio para disco e rede:** registrar o uso atual de `virtio-blk`, `virtio-scsi` e `virtio-net`; comparar alternativas apenas por requisitos de guest, número de discos ou medições. Aceite: drivers necessários são documentados e mudança de controlador não torna uma instalação existente incapaz de iniciar sem aviso e recuperação.
- [ ] **USB passthrough:** começar com um dispositivo e guest concretos, com seleção e liberação explícitas. Aceite: informar que o host pode perder acesso enquanto o dispositivo estiver entregue ao guest; testar desconexão física, reconexão e encerramento da Machine sem capturar automaticamente dispositivos de entrada do host.
- [ ] **Dispositivos extras:** selecionar por jornada concreta, em vez de um campo de argumentos QEMU arbitrários. Aceite: cada dispositivo tem suporte, permissões, persistência e comportamento de remoção documentados; recursos maduros são integrados no adapter.

#### Lifecycle e persistência

- [ ] **ACPI e shutdown/reboot:** auditar o caminho atual e avaliar agente guest quando disponível. Aceite: distinguir pedido enviado de desligamento concluído; timeout mantém estado verdadeiro e oferece Force Stop separado. Vincular à auditoria QEMU.
- [ ] **QMP, crash e recovery:** aproveitar a frente transversal para ampliar somente comandos/eventos necessários e reconciliar estado após falha do QEMU ou reinício do host. Aceite: PID ou socket antigo não basta para declarar ambiente ativo; recuperação não apaga discos nem entra em loop de reinício automático.
- [ ] **Autostart:** definir se significa login do usuário ou boot do host e quais ambientes participam. Aceite: opt-in por Machine, prevenção de start duplicado, falhas consultáveis e desativação simples; usar o gerenciador de serviços existente quando adequado, sem criar daemon por convenção.
- [ ] **Estado persistente:** revisar versões do registro, gravação e recuperação de interrupções, além do lock já previsto. Aceite: registros anteriores continuam legíveis ou têm migração documentada; corrupção é diagnosticada sem substituir silenciosamente o registro por uma lista vazia.

#### Performance avançada, condicionada a medições

- [ ] **Hugepages e CPU pinning:** investigar apenas para cargas reproduzíveis em que CPU/memória sejam gargalos. Aceite: ganho medido, custo para o host documentado, ausência de recursos tratada e reversão simples; não reservar recursos globalmente sem decisão explícita.
- [ ] **`io_uring`/AIO e cache modes:** verificar combinações suportadas pelo QEMU, armazenamento e filesystem do host. Aceite: comparar throughput, latência e integridade diante de falhas; documentar semântica de persistência e não adotar modos menos seguros como otimização automática.
- [ ] **NUMA:** manter como hipótese até haver host/carga que justifique topologia e afinidade específicas. Aceite: baseline e melhoria reproduzível superam a complexidade; hosts comuns não recebem configuração adicional por padrão.

#### Integração guest

- [ ] **Guest agent mais rico:** inventariar funções necessárias e suporte do QEMU Guest Agent e ferramentas existentes antes de propor componente próprio. Aceite: cada função expõe disponível/indisponível/não verificada e limitações por guest; atualizar o escopo se a proposta exigir um agente completo.
- [ ] **Resize e clipboard:** completar as validações já previstas em P2 e na frente QEMU. Aceite: resolução aplicada e texto trocado são confirmados em sessões suportadas, com desligamento da integração respeitado; não confundir clipboard de texto com transferência de arquivos.
- [ ] **File transfer bidirecional, opt-out:** selecionar mecanismo existente e uma combinação host/guest inicial, permitindo envio nos dois sentidos por padrão conforme a preferência do ambiente. Cada transferência parte de uma ação do usuário; não implica sincronização automática de diretórios nem compartilhamento irrestrito da home.
  - Aceite: testar host → guest e guest → host, destino, permissões, sobrescrita, cancelamento e arquivo parcial; desabilitar a capacidade bloqueia novas transferências e persiste após reabertura. Pasta compartilhada mantém suas próprias permissões e continua sendo uma alternativa compreensível.
- [ ] **Abrir links e aplicações entre host e guest, opt-out:** permitir encaminhar uma solicitação de abertura nos dois sentidos, habilitado por padrão por ambiente. Oferecer destinos identificáveis, como navegador do Omarchy ou aplicação de um ambiente específico; lembrar associações escolhidas explicitamente, sem substituir associações globais silenciosamente.
  - Aceite: testar um link do guest no navegador do host e um link/arquivo do host em uma aplicação do guest, além do sentido inverso para arquivos quando suportado. Cada abertura decorre de uma ação do usuário; não executar comandos arbitrários embutidos em URLs. Evitar ciclos de encaminhamento e ambiguidades entre ambientes, tratar destino indisponível e respeitar opt-out após reinício.
  - Dependências: abrir um arquivo do outro sistema exige transferência ou caminho compartilhado válido, com permissões e preferência próprias respeitadas; desabilitar transferência não pode ser contornado pelo fluxo de abertura. Usar handlers/protocolos existentes onde possível. Abrir uma aplicação dentro do guest não promete janela integrada ao host; Blend Mode de Machines continua sujeito à descoberta específica.
- [ ] **Exec em Machines:** definir autenticação/canal, contexto de usuário, diretório, ambiente, saída, código de retorno, timeout e cancelamento. Aceite: sem interpolação de shell implícita, comportamento programático documentado e erro claro quando o guest não oferece suporte; não presumir equivalência com Exec em Boxes.
- [ ] **Shutdown/reboot pelo guest:** integrar à mesma operação de lifecycle já auditada, com fallback documentado quando cabível. Aceite: ausência ou travamento do agente não bloqueia indefinidamente nem transforma shutdown em corte de energia.

#### Lacunas adicionais para descoberta

- [ ] **Firmware e boot:** avaliar BIOS/UEFI, persistência de NVRAM e ordem de boot por guest suportado; Secure Boot e TPM virtual apenas por requisito concreto. Aceite: configuração persiste e acompanha exportação/recuperação quando necessária; mudança incompatível não é aplicada silenciosamente.
- [ ] **Backup e restauração:** distinguir backup independente de snapshot dependente do disco original. Aceite: restauração comprovada em outro diretório/ambiente, com discos, configuração e estado de firmware necessários; explicitar consistência e necessidade de parar ou coordenar o guest.
- [ ] **Suspensão do host e estado salvo:** testar primeiro o comportamento existente ao suspender/retomar o notebook. Avaliar salvar/restaurar memória da Machine separadamente de Pause e snapshot de disco, apenas se houver demanda. Aceite: reconexão, relógio e rede são verificados; incompatibilidade de estado salvo após atualização tem recuperação documentada.
- [ ] **Compatibilidade após atualização:** definir versões de QEMU e guests efetivamente testadas, com atenção ao tipo de máquina virtual, firmware e dispositivos persistidos. Aceite: ambientes existentes iniciam após atualização suportada ou recebem diagnóstico e procedimento de recuperação; upgrades não trocam hardware virtual silenciosamente.

Sequência candidata após a auditoria: discos adicionais e crescimento, importação de discos e port forwarding simples; depois redes privadas, backup e integração guest orientada por uso. Bridge, passthrough, perfis e tuning avançado dependem de demanda e validação. Esta sequência deve ser repriorizada pelos achados de confiabilidade e pelos testes de usuário.

