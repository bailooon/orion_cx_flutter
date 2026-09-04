# Orion CX

**Plataforma de atendimento conversacional unificado — Claro Brasil · Challenge FIAP 2026**

Protótipo funcional que unifica App Claro, Web Portal e WhatsApp em uma única camada de orquestração: interpreta a intenção do cliente, mantém o contexto entre canais e transborda para um atendente humano quando a confiança da IA não é suficiente — sem o cliente perder o histórico.

```
Cliente ─┬─ App Claro ──┐
         ├─ Web Portal ─┼──► ORION Gateway ──► NLP · Auth · Call Mgmt · Notification
         └─ WhatsApp ───┘         │                        │
                                  └──► Redpanda (eventos) ─┴──► Dashboard do atendente
```

| Entregável | Onde |
|---|---|
| Arquitetura, diagramas e decisões | [ARQUITETURA.md](ARQUITETURA.md) |
| RF001–RF009 e RNF001–RNF008 mapeados ao código | [FUNCIONALIDADES.md](FUNCIONALIDADES.md) |
| Backend (Go, 5 microsserviços) | [`backend/`](backend) |
| Frontend (Flutter Web) | [`lib/`](lib) |
| Topologia local | [`docker-compose.yml`](docker-compose.yml) |

---

## Pré-requisitos

**Caminho completo (recomendado)**
- Docker Desktop com Docker Compose v2

**Caminho sem Docker**
- Go 1.25+ (backend)
- Flutter 3.24+ (frontend)

Nenhuma chave de API é obrigatória. Sem `ANTHROPIC_API_KEY`, o motor de NLU roda o classificador determinístico por regras e **os dois fluxos de demonstração funcionam integralmente**.

---

## Subir o ambiente

### Opção A — Docker Compose (stack completa)

```bash
docker compose up --build
```

Sobe PostgreSQL, Redis, Redpanda, os cinco serviços Orion, o job de carga de demonstração e o frontend.

- Frontend: <http://localhost:8080>
- API do gateway: <http://localhost:8000>
- Saúde do sistema: <http://localhost:8000/health>

Para usar o Claude no motor de NLU, crie um `.env` na raiz antes de subir:

```bash
ANTHROPIC_API_KEY=sk-ant-...
ORION_JWT_SECRET=troque-este-segredo
```

Derrubar tudo, incluindo os volumes:

```bash
docker compose down -v
```

### Opção B — sem Docker

O backend inteiro roda em um processo, com repositórios em memória e barramento interno — sem PostgreSQL, Redis ou Kafka instalados:

```bash
cd backend
go run ./cmd/orion -service=all
```

Em outro terminal:

```bash
flutter pub get
flutter run -d chrome --dart-define=ORION_API_URL=http://localhost:8000 --dart-define=ORION_WS_URL=ws://localhost:8000/ws
```

> A opção B é para desenvolvimento e para os testes. O caminho com persistência real em PostgreSQL é a opção A.

---

## Contas de demonstração

Criadas automaticamente pelo seed, já pré-preenchidas na tela de login.

| Perfil | E-mail | Senha |
|---|---|---|
| Cliente | `cliente@orion.dev` | `orion12345` |
| Atendente | `atendente@orion.dev` | `orion12345` |

O seed também cria três atendimentos de outros clientes (um na fila, um em andamento, um concluído), para que o dashboard não abra vazio. Ele é idempotente: reiniciar a stack não duplica dados.

---

## Rodar os dois fluxos de demonstração

### Fluxo A — suporte técnico com continuidade entre canais

1. Entre como **cliente** (`cliente@orion.dev`) e abra **Área do cliente**.
2. O canal ativo começa em **App Claro** — troque para **WhatsApp** no seletor de canal.
3. Envie **"minha internet está lenta"** (ou use o atalho *Internet lenta*).
   → A IA classifica `SUPORTE_TECNICO` com 94% de confiança e pergunta se pode reiniciar o sinal.
4. **Não responda.** Troque o canal para **Web Portal**.
   → O sistema reconhece o mesmo cliente, informa que a sessão foi recuperada e pergunta se você quer continuar de onde parou.
5. Clique em **Continuar aqui**.
   → A ação é executada, o atendimento é concluído e o chamado é encerrado junto.
6. Abra **Meus chamados** para ver o protocolo, a linha do tempo e as notificações geradas.

O histórico final mostra mensagens carimbadas em **dois canais diferentes** — é a prova da continuidade.

### Fluxo B — baixa confiança da IA com transbordo humano

1. Como **cliente**, envie **"quero contestar uma cobrança indevida"** (ou use o atalho *Cobrança indevida*).
   → Classificado como `CONTESTACAO_FATURA` com 45% de confiança, abaixo do limiar de 70%.
   → O sistema avisa que vai transferir e publica o evento `REQUIRED_HUMAN_ASSISTANCE`.
2. **Abra outra aba** em <http://localhost:8080> e entre como **atendente** (`atendente@orion.dev`).
3. Abra o **Dashboard administrativo**.
   → O atendimento aparece na fila **em tempo real**, com o resumo da IA, a intenção e a confiança.
4. Clique em **Assumir** e envie uma resposta manual.
5. Volte à aba do cliente.
   → A resposta do atendente está no mesmo canal em que o cliente estava, sem recarregar a página.
6. No dashboard, clique em **Concluir** para encerrar o atendimento.

> Para ver o tempo real de verdade, deixe as duas abas lado a lado. Elas compartilham o mesmo backend por WebSocket.

---

## Testes

**Backend** — inclui os dois fluxos de aceitação ponta a ponta, com os cinco serviços rodando de verdade:

```bash
cd backend && go test ./...
```

**Frontend**:

```bash
flutter test
```

O que cada teste garante está detalhado em [FUNCIONALIDADES.md](FUNCIONALIDADES.md#o-que-é-verificado-por-teste).

---

## Configuração

Todas as variáveis têm padrão seguro para desenvolvimento.

| Variável | Padrão | Para que serve |
|---|---|---|
| `ORION_ENV` | `development` | Em `production`, recusa iniciar com o segredo JWT padrão |
| `ORION_JWT_SECRET` | segredo de dev | Assinatura do token |
| `ORION_CONFIDENCE_THRESHOLD` | `0.70` | Abaixo disso, o atendimento vai para um humano |
| `ANTHROPIC_API_KEY` | vazio | Sem ela, o NLU usa o motor de regras |
| `ORION_NLU_MODEL` | `claude-opus-5` | Modelo usado na classificação |
| `ORION_POSTGRES_URL` | vazio | Sem ela, repositórios em memória |
| `ORION_REDIS_URL` | vazio | Sem ela, contexto de sessão em memória |
| `ORION_KAFKA_BROKER` | vazio | Sem ela, barramento de eventos em processo |
| `ORION_INTERNAL_TIMEOUT` | `3s` | Timeout entre serviços |
| `ORION_SEED` | `true` | Carga de demonstração no boot |

Lista completa em [`backend/internal/config/config.go`](backend/internal/config/config.go).

---

## API do gateway

Todas as rotas sob `/api` exigem `Authorization: Bearer <token>`, exceto login e cadastro.

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/api/auth/login` | Autentica e devolve o token |
| `POST` | `/api/auth/register` | Cria uma conta de cliente |
| `GET` | `/api/auth/me` | Perfil autenticado |
| `DELETE` | `/api/auth/me` | Eliminação de dados (LGPD) |
| `GET` | `/api/state?channel=` | Estado completo; abre a conversa no primeiro acesso |
| `GET` | `/api/session` | Contexto de jornada armazenado |
| `POST` | `/api/cases/{id}/messages` | Envia mensagem do cliente |
| `POST` | `/api/cases/{id}/switch-channel` | Troca de canal com recuperação de contexto |
| `POST` | `/api/cases/{id}/continue-here` | Retoma a ação pendente no canal atual |
| `POST` | `/api/cases/{id}/confirm-restart` · `/decline-restart` | Responde à ação pendente |
| `GET` | `/api/tickets` · `/api/notifications` · `/api/recommendations` | Chamados, avisos e sugestões |
| `POST` | `/api/cases/{id}/take` · `/agent-messages` · `/resolve` | **Somente atendente** |
| `GET` | `/ws?token=` | WebSocket de estado em tempo real |
| `GET` | `/health` · `/ready` | Saúde e prontidão |

---

## Estado da verificação

Transparência sobre o que foi executado no ambiente de desenvolvimento deste protótipo:

| Item | Status |
|---|---|
| Backend Go — build, `go vet`, suíte completa de testes | ✅ Executado, tudo passando |
| Fluxos A e B ponta a ponta | ✅ Executados por teste automatizado e manualmente via HTTP |
| Autenticação, autorização e mascaramento em log | ✅ Verificados |
| `docker compose up` | ⚠️ Não executado — Docker não estava instalado no ambiente de desenvolvimento |
| `flutter test` / `flutter analyze` | ⚠️ Não executados — Flutter não estava instalado no ambiente de desenvolvimento |
| Detector de corrida do Go (`-race`) | ⚠️ Não executado — exige compilador C, ausente no ambiente |

O backend foi validado rodando de verdade. O frontend e o Compose foram escritos com cuidado, mas precisam de uma execução em máquina com Flutter e Docker antes da entrega final.
