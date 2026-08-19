# Análise do documento e mapeamento para as telas

## 1. Requisitos identificados no documento

O material descreve uma solução de atendimento omnichannel composta por:

- canais de entrada: App Claro, Web Portal e WhatsApp;
- entrada unificada por Amazon API Gateway;
- microsserviços em Kubernetes / Amazon EKS;
- ORION Gateway como orquestrador;
- ORION Motor NLP / IA para análise de intenção;
- ORION Authenticator, Call Management e Notification;
- Kafka como barramento de eventos;
- DynamoDB, RDS e S3 como camada de persistência;
- SNS para notificações.

A interface precisava refletir principalmente duas jornadas.

### Jornada A - internet lenta

1. O cliente relata lentidão.
2. O backend cria ou recupera a sessão.
3. A IA identifica `SUPORTE_TECNICO`.
4. O estado e a ação pendente são persistidos.
5. O sistema pergunta se pode reiniciar o sinal.
6. O cliente pode trocar de canal.
7. O novo canal recupera o contexto e pergunta se deseja continuar.
8. Após a confirmação, a ação é executada e o cliente recebe o resultado.

### Jornada B - contestação de fatura

1. O cliente relata cobrança indevida.
2. A IA classifica `CONTESTACAO_FATURA` com 45% de confiança.
3. Por estar abaixo do limite de automação, o Gateway publica `REQUIRED_HUMAN_ASSISTANCE`.
4. A interface administrativa recebe o evento em tempo real.
5. Um atendente assume a conversa.
6. O dashboard solicita o histórico e recebe um resumo contextual.
7. O atendente envia uma resposta manual.
8. A resposta retorna ao mesmo canal do cliente.
9. O atendimento é concluído e o histórico permanece salvo.

## 2. Telas derivadas desses requisitos

| Tela | Origem no documento | Entrega no protótipo |
|---|---|---|
| Seleção de experiência | Não especificada; necessária para demonstrar os dois perfis sem autenticação real | Acesso independente à área do cliente e ao dashboard |
| Início do cliente | Canais de atendimento e início da interação | Atalhos para os dois cenários e sessão atual |
| Chat omnichannel | Fluxo do usuário, páginas 3 e 4 | Mensagens, intenção, ação pendente, confirmação e troca de canal |
| Sessões do cliente | Persistência de sessão e continuidade | Lista de protocolos e retomada do atendimento atual |
| Fila administrativa | Fluxo interno, páginas 5 e 6 | Alerta em tempo real, casos aguardando humano e ação de assumir |
| Workspace do atendente | Histórico, resumo da IA e resposta manual | Conversa completa, confiança, resumo, dados e envio de mensagem |
| Monitoramento | Arquitetura das páginas 1 a 3 | Representação visual dos componentes e eventos simulados |

## 3. Estado funcional implementado

O `OrionController` funciona como uma simulação local do ORION Gateway e da persistência. A área do cliente e a interface administrativa recebem a mesma instância do controlador. Isso permite demonstrar, sem backend externo, que:

- uma cobrança contestada pelo cliente aparece na fila administrativa;
- o atendente pode assumir o mesmo protocolo;
- a resposta manual aparece no histórico do cliente;
- uma ação pendente de suporte técnico sobrevive à troca de canal;
- o status da sessão muda entre automático, fila humana, em atendimento e concluído.

## 4. Premissas adotadas

O documento não fornece wireframes, identidade visual detalhada, regras de autenticação nem contratos completos de API. Por isso:

- a paleta é apenas inspirada na marca citada;
- a seleção de perfil substitui login e autorização;
- nomes, protocolos e planos são fictícios;
- apenas `POST /message` é tratado como rota explicitamente descrita;
- outros endpoints apresentados no README são propostas de integração;
- AWS, EKS, Kafka e bancos são representados por mocks locais.

## 5. Critérios de aceite cobertos

- Layout responsivo para celular, tablet e web/desktop.
- Navegação entre telas.
- Envio de mensagens.
- Respostas simuladas por intenção.
- Continuidade entre três canais.
- Confiança de 45% no cenário de contestação.
- Evento de transbordo humano.
- Fila administrativa compartilhada.
- Atribuição, resposta e conclusão pelo atendente.
- Ausência de dependências externas de estado.
