package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type Player struct {
	Nickname string
	Conn     net.Conn
	Move     string
}

type InviteData struct {
	FromNickname string `json:"from_nickname"`
}

type MoveData struct {
	Nickname string `json:"nickname"`
	Move     string `json:"move"`
}

var (
	players    = make(map[string]*Player) // Jogadores conectados
	mutex      = sync.Mutex{}             // Mutex para garantir consistência
	moveWait   = make(map[string]chan string)
	inviteWait = make(map[string]chan bool)
)

type OpponentRequest struct {
	ID        string
	Requester string
	Opponent  string
}

var (
	opponentRequests = make(map[string]*OpponentRequest) // Armazena requests pendentes
	requestCounter   = 0                                 // Contador simples para gerar IDs únicos
)

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Erro ao iniciar o servidor:", err)
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Println("Servidor escutando na porta 8080...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Erro ao aceitar conexão:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func generateRequestID() string {
	requestCounter++
	return fmt.Sprintf("req-%d", requestCounter)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Recebe a primeira mensagem do cliente (nickname + oponente)
	for {
		messageStr, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Erro ao ler mensagem:", err)
			return
		}

		var message Message
		err = json.Unmarshal([]byte(messageStr), &message)
		if err != nil {
			fmt.Println("Erro ao deserializar a mensagem:", err)
			return
		}

		fmt.Println("Mensagem recebida: ", message)

		switch message.Type {
		case "connect_request":
			var data map[string]string
			rawData, _ := json.Marshal(message.Data) // Converte interface{} para JSON
			json.Unmarshal(rawData, &data)           // Deserializa para map[string]string

			nickname := data["nickname"]
			handleConnectRequest(conn, nickname)
		case "opponent_request":
			var data map[string]string
			rawData, _ := json.Marshal(message.Data)
			json.Unmarshal(rawData, &data)

			nickname := data["nickname"]
			opponentNickname := data["opponent_nickname"]
			handleOpponentNickname(conn, nickname, opponentNickname)
		case "invite_response":
			handleInviteResponse(conn, message)
		}
	}
}

func handleInviteResponse(conn net.Conn, message Message) {
	// Deserializa os dados da resposta do convite
	var data map[string]interface{}
	rawData, _ := json.Marshal(message.Data)
	json.Unmarshal(rawData, &data)

	// Extrai o request_id e a resposta de aceitação/rejeição
	requestID := data["request_id"].(string)
	accepted := data["accepted"].(bool)

	mutex.Lock()
	defer mutex.Unlock()

	// Busca a solicitação pelo request_id
	opponentRequest, exists := opponentRequests[requestID]
	if !exists {
		// Se o ID da solicitação não for encontrado
		sendMessage(conn, Message{
			Type: "error",
			Data: map[string]string{
				"message": "Solicitação de oponente não encontrada.",
			},
		})
		return
	}

	// Verifica se o convite foi aceito
	if accepted {
		// Iniciar o jogo entre o jogador e o oponente
		player := players[opponentRequest.Requester]
		opponent := players[opponentRequest.Opponent]

		if player == nil || opponent == nil {
			// Caso algum jogador tenha desconectado
			sendMessage(conn, Message{
				Type: "error",
				Data: map[string]string{
					"message": "Jogador ou oponente desconectado.",
				},
			})
			return
		}

		// Inicia o jogo
		sendMessage(player.Conn, Message{
			Type: "game_start",
			Data: map[string]string{
				"message": "O jogo começou! Faça sua jogada: pedra, papel ou tesoura.",
			},
		})
		sendMessage(opponent.Conn, Message{
			Type: "game_start",
			Data: map[string]string{
				"message": "O jogo começou! Faça sua jogada: pedra, papel ou tesoura.",
			},
		})

		startGame(player, opponent)

	} else {
		// Envia a mensagem de rejeição ao jogador que fez o convite
		sendMessage(players[opponentRequest.Requester].Conn, Message{
			Type: "invite_rejected",
			Data: map[string]string{
				"message": "O jogador " + opponentRequest.Opponent + " recusou seu convite.",
			},
		})
	}

	// Remove a solicitação do map após o processamento
	delete(opponentRequests, requestID)
}

func handleConnectRequest(conn net.Conn, nickname string) {
	mutex.Lock()
	defer mutex.Unlock()

	// Registra o jogador na lista
	player := &Player{Nickname: nickname, Conn: conn}
	players[nickname] = player

	// Responde ao jogador que a conexão foi bem-sucedida
	sendMessage(conn, Message{
		Type: "connect_response",
		Data: map[string]string{
			"status":  "success",
			"message": "Usuário conectado com sucesso.",
		},
	})
}

func handleOpponentNickname(conn net.Conn, nickname, opponentNickname string) {
	mutex.Lock()

	// Verifica se o oponente existe
	opponent, exists := players[opponentNickname]
	if !exists {
		// Envia mensagem de erro
		sendMessage(conn, Message{
			Type: "opponent_response",
			Data: map[string]string{
				"status":  "error",
				"message": "Player not found.",
			},
		})
		mutex.Unlock()
		return
	}

	// Gerar um id único para esta solicitação
	requestID := generateRequestID()

	// Cria a solicitação e armazena no map
	opponentRequest := &OpponentRequest{
		ID:        requestID,
		Requester: nickname,
		Opponent:  opponentNickname,
	}
	opponentRequests[requestID] = opponentRequest

	// Envia o convite para o oponente, incluindo o requestID
	sendMessage(opponent.Conn, Message{
		Type: "invite_request",
		Data: map[string]string{
			"from_nickname": nickname,
			"request_id":    requestID, // Enviar o ID da solicitação
		},
	})

	// Cria um canal para aguardar a resposta do convite
	inviteChan := make(chan bool)
	inviteWait[opponentNickname] = inviteChan

	mutex.Unlock()

	fmt.Println("Solicitação de oponente enviada. requestID:", requestID)

	select {
	case accepted := <-inviteChan:
		if accepted {
			// Inicia o jogo
			sendMessage(conn, Message{
				Type: "game_start",
				Data: map[string]string{
					"message": "O jogo começou! Faça sua jogada: pedra, papel ou tesoura.",
				},
			})
			sendMessage(opponent.Conn, Message{
				Type: "game_start",
				Data: map[string]string{
					"message": "O jogo começou! Faça sua jogada: pedra, papel ou tesoura.",
				},
			})

			startGame(players[nickname], opponent)

		} else {
			// Envia mensagem de rejeição
			sendMessage(conn, Message{
				Type: "invite_rejected",
				Data: map[string]string{
					"message": "Player " + opponentNickname + " rejected your invite.",
				},
			})
		}
	case <-time.After(120 * time.Second):
		// Timeout de resposta
		sendMessage(conn, Message{
			Type: "invite_rejected",
			Data: map[string]string{
				"message": "Invite timed out.",
			},
		})
	}
}

func startGame(player1, player2 *Player) {
	moveChan1 := make(chan string)
	moveChan2 := make(chan string)
	moveWait[player1.Nickname] = moveChan1
	moveWait[player2.Nickname] = moveChan2

	go waitForMove(player1, moveChan1)
	go waitForMove(player2, moveChan2)

	select {
	case move1 := <-moveChan1:
		player1.Move = move1
	case <-time.After(90 * time.Second):
		endGameDueToTimeout(player1, player2)
		return
	}

	select {
	case move2 := <-moveChan2:
		player2.Move = move2
	case <-time.After(90 * time.Second):
		endGameDueToTimeout(player1, player2)
		return
	}

	determineWinner(player1, player2)
}

func waitForMove(player *Player, moveChan chan string) {
	reader := bufio.NewReader(player.Conn)
	for {
		messageStr, _ := reader.ReadString('\n')
		var message Message
		_ = json.Unmarshal([]byte(messageStr), &message)

		if message.Type == "move" {
			data := message.Data.(map[string]interface{})
			move := data["move"].(string)
			moveChan <- move
			return
		}
	}
}

func determineWinner(player1, player2 *Player) {
	result := ""
	if player1.Move == player2.Move {
		result = "It's a tie! Play again."
		sendMessage(player1.Conn, Message{
			Type: "game_result",
			Data: map[string]string{
				"player1_move": player1.Move,
				"player2_move": player2.Move,
				"message":      result,
			},
		})
		sendMessage(player2.Conn, Message{
			Type: "game_result",
			Data: map[string]string{
				"player1_move": player1.Move,
				"player2_move": player2.Move,
				"message":      result,
			},
		})
		startGame(player1, player2) // Recomeça o jogo no caso de empate
		return
	}

	winner := determineWinnerLogic(player1.Move, player2.Move)

	result = fmt.Sprintf("Player %s wins!", winner)
	sendMessage(player1.Conn, Message{
		Type: "game_result",
		Data: map[string]string{
			"player1_move": player1.Move,
			"player2_move": player2.Move,
			"winner":       winner,
		},
	})
	sendMessage(player2.Conn, Message{
		Type: "game_result",
		Data: map[string]string{
			"player1_move": player1.Move,
			"player2_move": player2.Move,
			"winner":       winner,
		},
	})

	// Após o jogo, desconectar os clientes
	player1.Conn.Close()
	player2.Conn.Close()
}

func determineWinnerLogic(move1, move2 string) string {
	switch move1 {
	case "rock":
		if move2 == "scissors" {
			return "player1"
		} else if move2 == "paper" {
			return "player2"
		}
	case "paper":
		if move2 == "rock" {
			return "player1"
		} else if move2 == "scissors" {
			return "player2"
		}
	case "scissors":
		if move2 == "paper" {
			return "player1"
		} else if move2 == "rock" {
			return "player2"
		}
	}
	return "tie"
}

func endGameDueToTimeout(player1, player2 *Player) {
	message := "The game has ended due to inactivity."
	sendMessage(player1.Conn, Message{
		Type: "timeout",
		Data: map[string]string{
			"message": message,
		},
	})
	sendMessage(player2.Conn, Message{
		Type: "timeout",
		Data: map[string]string{
			"message": message,
		},
	})
	player1.Conn.Close()
	player2.Conn.Close()
}

func sendMessage(conn net.Conn, message Message) {
	messageBytes, _ := json.Marshal(message)
	conn.Write(append(messageBytes, '\n')) // Envia a mensagem com nova linha
}
