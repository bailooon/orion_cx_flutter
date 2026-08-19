# Orion CX — protótipo funcional em Flutter

Aplicativo demonstrativo criado a partir do documento técnico **Sprint 2: Orion CX**. O projeto cobre as duas experiências descritas no material: a jornada conversacional do cliente e a interface administrativa usada no transbordo para atendimento humano.

## O que foi implementado

### Área do cliente

- Tela inicial com atalhos para os cenários de **internet lenta** e **contestação de cobrança**.
- Chat conversacional com mensagens de cliente, Orion IA, atendente e eventos de sistema.
- Classificação simulada de intenção:
  - `SUPORTE_TECNICO` com 94% de confiança;
  - `CONTESTACAO_FATURA` com 45% de confiança.
- Confirmação funcional do reinício de sinal.
- Continuidade entre **App Claro**, **Web Portal** e **WhatsApp**, incluindo recuperação de ação pendente.
- Transferência para atendimento humano quando a confiança é baixa.
- Tela de sessões e protocolos.

### Interface administrativa

- Dashboard responsivo com fila, atendimentos ativos, concluídos e eventos não lidos.
- Alerta visual para `REQUIRED_HUMAN_ASSISTANCE`.
- Fila de clientes atualizada pelo mesmo estado usado na área do cliente.
- Ação de assumir atendimento.
- Workspace do atendente com:
  - histórico completo;
  - resumo contextual da IA;
  - intenção e nível de confiança;
  - dados básicos do cliente;
  - resposta manual;
  - conclusão do atendimento.
- Tela de monitoramento que representa os componentes da arquitetura descrita no documento: canais, API Gateway, cluster EKS, microsserviços Orion, Kafka e persistência AWS.

## Como testar os dois fluxos principais

### Fluxo 1 — internet lenta e continuidade de canal

1. Entre em **Área do cliente**.
2. Selecione **Internet lenta**.
3. Antes de confirmar, altere o canal para **Web Portal**.
4. O chat recuperará a ação pendente e perguntará se deseja continuar.
5. Selecione **Continuar aqui** para executar o reinício simulado.

### Fluxo 2 — contestação de fatura e transbordo humano

1. Entre em **Área do cliente**.
2. Selecione **Cobrança indevida**.
3. O caso será classificado com 45% de confiança e entrará na fila humana.
4. Volte à seleção de experiência e abra o **Dashboard administrativo**.
5. Selecione o protocolo do cliente, clique em **Assumir** e envie uma resposta.
6. Ao retornar à área do cliente, a mensagem do atendente estará no mesmo histórico.

## Executar o projeto

A pasta `web/` já está configurada. Pré-requisitos: Flutter 3.22 ou mais recente instalado e um dispositivo, emulador ou navegador configurado.

```bash
flutter pub get
flutter run -d chrome
```

Para executar os testes:

```bash
flutter test
```

Para gerar também as pastas nativas Android e iOS, execute `flutter create . --platforms=android,ios` na raiz do projeto.

O projeto não usa pacotes externos de estado ou interface. Toda a demonstração funciona apenas com o SDK do Flutter.

## Estrutura principal

```text
lib/
├── main.dart
├── app.dart
├── core/
│   ├── app_theme.dart
│   ├── models.dart
│   └── orion_controller.dart
├── screens/
│   ├── role_selection_screen.dart
│   ├── customer_portal.dart
│   └── admin_portal.dart
└── widgets/
    └── common_widgets.dart
```

## Estado e integração

O arquivo `lib/core/orion_controller.dart` atua como backend em memória. Ele simula:

- criação e manutenção de sessão;
- análise de intenção;
- persistência de contexto;
- publicação de evento de transbordo;
- atribuição de atendimento;
- envio de mensagem manual;
- conclusão do protocolo.

Para uma integração real, o controlador pode ser substituído por um repositório que converse com o ORION Gateway. O documento explicita o envio inicial por `POST /message`; os demais contratos abaixo são sugestões de implementação para completar a interface, não endpoints definidos no material original:

```text
POST /message
GET  /sessions/{sessionId}
GET  /admin/cases
POST /admin/cases/{caseId}/assign
POST /admin/cases/{caseId}/messages
POST /admin/cases/{caseId}/resolve
WS   /admin/events
```

## Decisões de escopo

- A autenticação aparece na arquitetura, mas o documento não define telas ou regras de login. Por isso, o protótipo começa com uma seleção de experiência.
- AWS, Kubernetes, Kafka, DynamoDB, RDS, S3 e SNS não são acessados. A tela de monitoramento é visual e seus indicadores são explicitamente marcados como simulados.
- Nomes de clientes, planos e protocolos são dados fictícios de demonstração.
- O visual usa uma paleta inspirada na marca citada no documento, sem depender de arquivos de logo ou fontes proprietárias.
