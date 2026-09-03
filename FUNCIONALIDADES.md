# Orion CX — Funcionalidades e requisitos

Este documento liga cada requisito do desafio à parte do sistema que o implementa, e explica **por que** cada escolha foi feita. Os caminhos são clicáveis e apontam para o código real.

A arquitetura completa está em [ARQUITETURA.md](ARQUITETURA.md). Como executar está no [README.md](README.md).

---

## Sumário

- [Requisitos funcionais](#requisitos-funcionais)
- [Requisitos não funcionais](#requisitos-não-funcionais)
- [LGPD](#lgpd)
- [O que é verificado por teste](#o-que-é-verificado-por-teste)
- [Limites conhecidos](#limites-conhecidos-desta-versão)

---

## Requisitos funcionais

### RF001 — Autenticação multicanal

**Onde:** [`internal/authsvc`](backend/internal/authsvc), [`internal/platform/security`](backend/internal/platform/security/security.go), [`lib/screens/login_screen.dart`](lib/screens/login_screen.dart)

O ORION Authenticator emite um **JWT HS256** com expiração. Como a validação é feita pela assinatura, qualquer serviço valida o token sem consultar o banco — e o mesmo token vale no App, no Web Portal e no WhatsApp.

O token é o que amarra o RF001 ao RF003: é dele que sai o `user_id` usado para recuperar o contexto quando o cliente troca de canal. Sem identidade única, não existe jornada contínua.

**Por que assim:**
- **JWT em vez de sessão em banco** — validação stateless, então escalar o gateway não exige um store de sessões compartilhado.
- **bcrypt com custo configurável** — o hash é lento por construção; o custo sobe conforme o hardware melhora, sem migrar dados.
- **Mesma resposta 401 para usuário inexistente e senha errada** — mensagens diferentes permitiriam enumerar contas válidas.
- **Cadastro público força `role = customer`** ([`handler.go`](backend/internal/gatewaysvc/handler.go)) — pedir `role: "agent"` na API pública devolve uma conta de cliente. Contas de atendente só nascem pela rede interna.
- **Algoritmo fixado na verificação** — a assinatura só é aceita se for HMAC, o que bloqueia o ataque clássico de `alg: none`.

No frontend, `LoginScreen` é a única tela alcançável sem sessão; [`app.dart`](lib/app.dart) faz esse bloqueio.

---

### RF002 — Histórico de interações

**Onde:** [`internal/callsvc`](backend/internal/callsvc), tabelas `calls.conversations` e `calls.messages`

Toda mensagem é persistida com **ator** (`customer` / `assistant` / `agent` / `system`), **texto**, **canal de origem** e **horário**. A consulta do histórico é uma leitura ordenada por `(conversation_id, sent_at)`, coberta por índice.

**Por que assim:**
- **`channel` fica na mensagem, não na conversa.** É a única forma de provar que um atendimento atravessou canais — e é exatamente o que o Fluxo A demonstra.
- **Mensagens de sistema fazem parte do histórico.** Trocas de canal, transbordos e atribuições viram registros visíveis, não apenas linhas de log. O atendente vê o que aconteceu, não só o que foi dito.
- **Carregamento em duas consultas.** Listar conversas e depois buscar todas as mensagens com `WHERE conversation_id = ANY($1)` evita o problema N+1 na fila do dashboard.

---

### RF003 — Continuidade de jornada entre canais

**Onde:** [`internal/platform/cache`](backend/internal/platform/cache/cache.go), [`gatewaysvc/service.go`](backend/internal/gatewaysvc/service.go) (`SwitchChannel`, `ActiveConversation`)

Este é o requisito central do produto. O contexto da jornada — conversa ativa, último canal, ação pendente, última intenção — fica em um store de baixa latência **indexado por `user_id`**.

Quando o cliente aparece em outro canal:

1. O gateway lê o contexto pela identidade, não pelo canal.
2. Registra no histórico uma mensagem de sistema: *"Sessão CX-… recuperada de WhatsApp em Web Portal."*
3. Se havia ação pendente, converte `RESTART_SIGNAL` em `CONTINUE_PENDING_ACTION` e **pergunta se o cliente quer retomar** em vez de executar sozinho.

**Por que assim:**
- **A chave é o `user_id`.** Qualquer outra chave (sessão, dispositivo, canal) quebraria a continuidade justamente no momento em que ela importa.
- **Retomar é uma pergunta, não uma ação automática.** O cliente pediu o reinício em outro contexto e pode ter desistido; executar sem confirmar seria agir por conta própria.
- **TTL de 24 h.** Contexto velho demais não é contexto, é ruído.
- **Contingência sem Redis.** Se o store estiver fora, o gateway busca a conversa aberta no Call Management. Perde-se uma leitura rápida, não a jornada.

---

### RF004 — Interpretação de intenção via IA

**Onde:** [`internal/nlpsvc`](backend/internal/nlpsvc) — [`llm.go`](backend/internal/nlpsvc/llm.go) e [`rules.go`](backend/internal/nlpsvc/rules.go)

Dois motores atrás de um contrato único que devolve `intent`, `confidence`, `summary` e `engine`:

| Motor | Quando roda |
|---|---|
| **Claude** (SDK oficial Go, saída restrita por JSON Schema) | Quando há `ANTHROPIC_API_KEY` |
| **Regras determinísticas** | Sem chave, ou quando o modelo falha, estoura o tempo, devolve JSON inválido ou recusa |

**Por que assim:**
- **Saída estruturada por schema** em vez de pedir "responda em JSON" — o serviço nunca precisa interpretar prosa.
- **Esforço baixo na chamada** — classificar é tarefa rasa; isso mantém a latência dentro do orçamento sem mudar a resposta.
- **Timeout curto com contingência** — o cliente recebe resposta mesmo quando o modelo não responde a tempo.
- **O motor de regras não é enfeite.** Normaliza acentos (`"está lenta"` e `"esta lenta"` casam na mesma regra), exige combinação de termos (`internet` **e** `lenta`), casa palavras curtas por palavra inteira (`"nao"` não dispara dentro de `"naoperacional"`) e só aceita `sim`/`não` quando há pergunta pendente.
- **O campo `engine` viaja na resposta** — nos logs e na UI dá para separar uma decisão do modelo de uma decisão de contingência.
- **A mensagem do cliente nunca vai para o log**, nem em caso de erro. Só metadados.

---

### RF005 — Roteamento automático

**Onde:** [`internal/gatewaysvc/dialogue.go`](backend/internal/gatewaysvc/dialogue.go)

A função `decide` transforma classificação + estado em uma decisão de roteamento, avaliada nesta ordem:

1. **Resposta a pergunta pendente** — `sim`/`não` se referem à ação já proposta, não a uma intenção nova.
2. **Intenção sempre humana** — `CONTESTACAO_FATURA` e `CANCELAMENTO` vão para uma pessoa, **em qualquer nível de confiança**.
3. **Confiança abaixo do limiar** — primeira mensagem ambígua gera pergunta de esclarecimento; a segunda escala.
4. **Intenção automatizável e confiante** — o assistente resolve.

**Por que assim:**
- **Dois guardas independentes.** Confiança alta não é o mesmo que decisão segura: contestação mexe no dinheiro do cliente e cancelamento mexe no contrato. Um erro nesses casos é caro de desfazer, então a política de negócio prevalece sobre o score.
- **Uma pergunta antes de escalar.** Mandar alguém para a fila por causa de uma frase vaga é pior do que perguntar. O contador de turnos sem classificação fica no contexto de sessão, então funciona mesmo se o cliente trocar de canal no meio.
- **A ordem importa.** Se a checagem de intenção viesse antes da resposta pendente, um `"sim"` seria classificado como intenção nova e a jornada travaria.
- **O motivo do transbordo é escrito no histórico**, com intenção, confiança e limiar. O atendente entende por que o caso chegou até ele.

---

### RF006 — Chamados (tickets)

**Onde:** [`callsvc/service.go`](backend/internal/callsvc/service.go), tabelas `calls.tickets` e `calls.ticket_events`; [`lib/screens/tickets_screen.dart`](lib/screens/tickets_screen.dart)

Um chamado nasce automaticamente da conversa que o originou, com protocolo, categoria (a intenção classificada), canal de abertura e uma linha do tempo.

**Por que assim:**
- **O chamado segue o status da conversa.** Qualquer turno que mude o status da conversa sincroniza os chamados ligados a ela (`ApplyTurn` → `syncTickets`). Sem isso, resolver a conversa por um caminho deixaria o protocolo aberto para sempre — foi exatamente o bug encontrado durante a verificação manual e corrigido com o teste `TestTicketFollowsConversationStatus`.
- **O protocolo é criado depois do turno confirmado**, nunca antes. Um chamado não pode apontar para um estado que foi revertido.
- **A linha do tempo é apenas anexada**, nunca reescrita — é o que torna o acompanhamento auditável.

---

### RF007 — Recomendações personalizadas

**Onde:** [`gatewaysvc/operations.go`](backend/internal/gatewaysvc/operations.go) (`Recommendations`)

As sugestões são derivadas do histórico real do cliente: quantos atendimentos técnicos ele abriu, se já questionou uma cobrança, se demonstrou interesse em consumo ou upgrade, se tem atendimento em andamento.

**Por que assim:**
- **Nada é aleatório.** Cada cartão declara o fato que o gerou, e a UI mostra essa justificativa ("Por que você está vendo isso: …"). Uma recomendação que não se explica é publicidade; uma que se explica é atendimento.
- **Calculado sob demanda a partir do próprio histórico**, sem perfil paralelo. Menos dado guardado, mesma utilidade.
- **Sempre há um cartão.** Cliente sem pendências recebe um cartão explicando a multicanalidade, em vez de uma tela vazia.

---

### RF008 — Multicanalidade unificada

**Onde:** [`internal/domain/domain.go`](backend/internal/domain/domain.go) (`Channel`), gateway como entrada única, [`lib/screens/customer_portal.dart`](lib/screens/customer_portal.dart)

App Claro, Web Portal e WhatsApp são **um canal declarado em cada requisição**, não três backends. O mesmo endpoint atende os três; o canal muda a apresentação e o carimbo da mensagem, nunca a regra de negócio.

**Por que assim:**
- **Um único ponto de entrada.** É o papel do Amazon API Gateway na arquitetura-alvo e do ORION Gateway aqui. Autenticação, CORS, correlação de requisição e políticas ficam em um lugar só.
- **O dashboard vê todos os atendimentos, não só a fila.** Conversas que o assistente está conduzindo sozinho aparecem na aba *Com a IA*, com o histórico completo. O atendente pode **intervir** numa delas em vez de esperar o cliente se frustrar até disparar um transbordo — a conversa é preservada, a ação pendente do assistente é descartada e o cliente recebe um aviso de que uma pessoa entrou.
- **Canal validado no domínio.** Um canal desconhecido é rejeitado com 400 em vez de gravar lixo no histórico.
- **O frontend simula os três canais** com um seletor, para que a troca de canal seja demonstrável em uma tela só — mas a troca é real: gera requisição, muda o registro no banco e reidrata o contexto.

---

### RF009 — Notificações

**Onde:** [`internal/notifysvc`](backend/internal/notifysvc)

O ORION Notification **consome eventos do barramento** — não é chamado por ninguém. Transbordo, atribuição, resposta do atendente, abertura e mudança de chamado viram notificações persistidas e entregues ao cliente.

**Por que assim:**
- **Orientado a eventos.** Notificar não pode atrasar a resposta ao cliente: o gateway publica e responde. Um consumidor lento atrasa o aviso, não a conversa.
- **`CONVERSATION_UPDATED` é ignorado de propósito.** Notificar cada turno seria spam; só marcos de atendimento viram aviso.
- **Persistidas antes de entregues** — a notificação sobrevive ao cliente estar offline, e é isso que a aba de avisos mostra ao voltar.
- **É onde o Amazon SNS entraria** em produção, no mesmo ponto do código.

---

## Requisitos não funcionais

### RNF001 — Resposta abaixo de 2 s

**Como foi tratado:**
- Chamada ao modelo com **timeout curto** e contingência imediata por regras — o pior caso do NLU é limitado, não indefinido.
- **Timeout de 3 s e 2 tentativas** em toda chamada entre serviços ([`httpx.Client`](backend/internal/platform/httpx/httpx.go)); `4xx` não é repetido, por ser resposta determinística.
- **Um turno = uma transação.** O gateway envia mensagens e mudança de estado em uma única chamada ao Call Management, em vez de três idas e voltas.
- **Índices no caminho quente** e carregamento em duas consultas, sem N+1.
- **Latência medida e registrada** por requisição no log de acesso.

Sem chave de API, os dois fluxos rodam com o motor de regras e a resposta é praticamente instantânea.

---

### RNF002 — Disponibilidade

Não é mensurável em um MVP local. O que foi implementado como evidência do princípio:

- **`/health`** em todos os serviços e **`/ready`** no gateway, que falha enquanto uma dependência dura estiver fora — um orquestrador não roteia tráfego para instância quebrada.
- **Health checks + `restart: unless-stopped`** em todos os contêineres.
- **Ordem de subida declarada** com `condition: service_healthy`.
- **Middleware de recuperação de `panic`**: um handler quebrado devolve 500, não derruba o processo.
- **Encerramento gracioso** com `SIGTERM` e drenagem de conexões em até 10 s.
- **Espera pelo banco no boot**, com retry por até 30 s — é o que faz `docker compose up` funcionar em volume novo.

---

### RNF003 — Escalabilidade

Justificada em detalhe na [seção 9 de ARQUITETURA.md](ARQUITETURA.md#9-escalabilidade-rnf003). Em resumo: serviços sem estado, contexto compartilhado em store de baixa latência com chave O(1), escrita crítica isolada por conversa, notificação assíncrona, índices no caminho de leitura — e um gargalo conhecido e declarado no WebSocket.

---

### RNF004 — Segurança

| Controle | Implementação |
|---|---|
| Senha | bcrypt, custo configurável, hash nunca serializado |
| Token | JWT HS256, expiração obrigatória, emissor validado, algoritmo fixado |
| Autorização | Middleware de perfil no dashboard + checagem de propriedade da conversa |
| Escalação de privilégio | Cadastro público não concede perfil de atendente |
| Segredos | Somente por variável de ambiente; `ORION_ENV=production` recusa iniciar com o segredo padrão |
| Superfície | Apenas o gateway é publicado no host |
| Corpo de requisição | Limite de 1 MB por payload |
| CORS | Origens configuráveis por ambiente |
| Erros | Envelope único, sem vazar detalhe interno; erros desconhecidos viram 500 genérico |
| Rastreabilidade | `X-Request-Id` propagado entre serviços |

---

### RNF005 — Interface responsiva

O frontend Flutter usa `LayoutBuilder` para alternar entre layout de coluna e de duas colunas conforme a largura, com pontos de quebra em ~840 px e ~920 px. Todas as telas — login, seleção, portal do cliente, chamados e dashboard — funcionam em celular, tablet e desktop.

---

### RNF006 — Usabilidade

- **Uma decisão por tela.** Login → área → conversa.
- **O estado é visível.** Intenção, confiança e status aparecem na conversa; o cliente vê por que foi transferido.
- **A troca de canal é um clique**, e o sistema anuncia o que recuperou.
- **Ações pendentes viram botões**, não instruções para digitar.
- **Contas de demonstração pré-preenchidas** na tela de login, para que os dois fluxos sejam executáveis sem consultar documentação.

---

### RNF007 — Tolerância a falhas

Cada dependência tem um comportamento definido em falha — a tabela completa está na [seção 10 de ARQUITETURA.md](ARQUITETURA.md#10-tolerância-a-falhas-rnf007).

O princípio: **nenhuma falha de infraestrutura pode deixar o cliente sem resposta.** Modelo fora → regras. Serviço NLP fora → regras no próprio gateway. Redis fora → consulta ao Call Management. Kafka fora → perde-se a notificação, não a conversa.

---

### RNF008 — Manutenibilidade

- **Go tipado, sem `any` no domínio**; entidades em um pacote só, compartilhadas pelos cinco serviços.
- **Repositórios atrás de interface**, com duas implementações (PostgreSQL e memória) que passam pelos mesmos testes.
- **Plataforma isolada em `internal/platform`** — banco, cache, barramento, HTTP, WebSocket, segurança e log. Trocar Redis por DynamoDB é trocar uma implementação.
- **`gofmt` e `go vet` limpos** em todo o repositório.
- **Comentários explicam o porquê**, não o quê: cada decisão não óbvia tem a razão registrada onde ela vale.
- **Testes** cobrindo os dois fluxos de aceitação ponta a ponta, o motor de NLU, as regras de roteamento, o ciclo de vida de conversas e chamados, autenticação/autorização e mascaramento de dados pessoais.

---

## LGPD

Os dados são fictícios, mas tratados como se fossem reais.

| Princípio | Implementação |
|---|---|
| **Não exposição em log** | Um `slog.Handler` próprio ([`logging.go`](backend/internal/platform/logging/logging.go)) mascara CPF, e-mail e telefone e substitui campos sensíveis (`password`, `token`, `email`, `text`, `body`) por `[REDACTED]` **antes** de o registro sair. A proteção é do logger, não da disciplina de quem chama — inclusive a mensagem do cliente nunca é registrada |
| **Minimização** | O documento já é armazenado mascarado (`***.482.***-**`). O barramento carrega metadados e, no máximo, um trecho curto — nunca a mensagem inteira |
| **Direito de eliminação** | `DELETE /api/auth/me` apaga conversas, mensagens, chamados e notificações; remove o contexto de sessão; anonimiza o cadastro |
| **Anonimização em vez de exclusão do cadastro** | A linha do usuário sobrevive com `anonymized_at` preenchido e identificadores zerados. Os protocolos continuam auditáveis e a conta anonimizada não consegue mais autenticar |
| **Confidencialidade** | Senhas em bcrypt, token assinado com expiração, HTTPS previsto em produção, segredos fora do código |

---

## O que é verificado por teste

Executar com `cd backend && go test ./...`:

| Teste | O que garante |
|---|---|
| `TestFlowA_TechnicalSupportAcrossChannels` | Fluxo A inteiro: classificação confiante, ação pendente, troca de canal com recuperação de contexto, execução, histórico único cobrindo dois canais e chamado encerrado junto |
| `TestFlowB_LowConfidenceHumanHandoff` | Fluxo B inteiro: confiança abaixo do limiar, transbordo, fila no dashboard, cliente barrado com 403, atribuição, resposta manual no canal de origem, encerramento e notificações |
| `TestAgentCanInterveneInAutomatedConversation` | O atendente entra numa conversa que ainda está com a IA, o histórico é preservado e a ação pendente do assistente é descartada |
| `TestResolvedConversationCannotBeAssigned` | Conversa concluída não volta para atendimento; conversa já atribuída não é reatribuída |
| `TestDashboardReceivesHandoffInRealTime` | O dashboard recebe o transbordo pelo WebSocket, sem polling e sem recarregar, já com resumo da IA e confiança abaixo do limiar |
| `TestWebSocketRequiresAValidToken` | O canal de tempo real não é uma porta sem autenticação: handshake sem token ou com token inválido devolve 401 |
| `TestCustomerSocketOnlySeesOwnConversations` | Um socket de cliente nunca recebe a conversa de outro cliente |
| `TestUnclearMessageAsksBeforeEscalating` | Primeira mensagem ambígua pergunta; a segunda escala |
| `TestLGPDErasure` | Eliminação apaga os dados e impede novo login |
| `TestPublicRegistrationCannotCreateAgents` | Cadastro público não concede perfil de atendente |
| `TestRuleClassifierIntents` | 12 casos de classificação, incluindo as duas frases de aceitação, e a fronteira do limiar de automação |
| `TestAffirmativeIgnoredWithoutPendingQuestion` | Um `"sim"` solto não executa ação nenhuma |
| `TestEveryDecisionAnswersTheCustomer` | Varre 288 combinações de intenção × estado × confiança e garante que nenhuma deixa o cliente sem resposta |
| `TestDecideAlwaysEscalatesBillingDisputes` | Contestação vai para humano mesmo com 99% de confiança |
| `TestTicketFollowsConversationStatus` | O chamado nunca fica aberto depois de a conversa ser resolvida |
| `TestConcurrentTurnsDoNotLoseMessages` | 20 escritas simultâneas na mesma conversa não perdem mensagem |
| `TestReturnedConversationIsACopy` | Estado interno não é mutável através do que foi devolvido |
| `TestPasswordIsHashedNotStored`, `TestTokenRoundTrip`, `TestExpiredTokenIsRejected`, `TestTamperedTokenIsRejected`, `TestRequireRole` | Base de RNF004 |
| `TestHandlerRedactsAttributes` | Log não vaza e-mail, senha nem CPF, e preserva o protocolo para suporte |

No frontend, `flutter test` cobre o bloqueio de acesso sem sessão, o cenário de internet lenta, o dashboard do atendente, a tela de chamados e a exibição de erro de credenciais.

---

## Limites conhecidos desta versão

Declarados por honestidade de escopo — cada um tem o caminho de produção descrito na [seção 7 de ARQUITETURA.md](ARQUITETURA.md#7-simplificações-assumidas).

1. **Comunicação interna em HTTP/JSON**, não gRPC.
2. **Notificação simulada** — sem push, SMS ou e-mail reais.
3. **Sem armazenamento de objetos (S3/MinIO)** — nenhum fluxo desta versão envia anexo.
4. **Token não persistido no navegador** — recarregar a página exige novo login.
5. **Snapshot completo por WebSocket** em vez de delta — suficiente para dezenas de atendentes, não para milhões de clientes.
6. **Sem rate limiting nem WAF** — em produção seriam responsabilidade do Amazon API Gateway.
7. **Um schema por serviço em um único PostgreSQL**, em vez de um banco por serviço.
