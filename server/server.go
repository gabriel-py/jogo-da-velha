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

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Cria um leitor para ler mensagens do cliente
	reader := bufio.NewReader(conn)

	// Recebe a primeira mensagem do cliente (nickname + oponente)
	messageStr, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Erro ao ler mensagem:", err)
		return
	}

	// Deserializa a mensagem
	var message Message
	err = json.Unmarshal([]byte(messageStr), &message)
	if err != nil {
		fmt.Println("Erro ao deserializar a mensagem:", err)
		return
	}

	// Lida com a solicitação de conexão
	if message.Type == "connect_request" {
		data := message.Data.(map[string]interface{})
		nickname := data["nickname"].(string)
		opponentNickname := data["opponent_nickname"].(string)

		handleConnectRequest(conn, nickname, opponentNickname)
	}
}

func handleConnectRequest(conn net.Conn, nickname, opponentNickname string) {
	mutex.Lock()
	// Verifica se o oponente existe
	opponent, exists := players[opponentNickname]
	if !exists {
		// Envia mensagem de erro
		sendMessage(conn, Message{
			Type: "connect_response",
			Data: map[string]string{
				"status":  "error",
				"message": "Player not found.",
			},
		})
		mutex.Unlock()
		return
	}

	// Registra o jogador na lista
	player := &Player{Nickname: nickname, Conn: conn}
	players[nickname] = player
	mutex.Unlock()

	// Envia convite para o oponente
	sendMessage(opponent.Conn, Message{
		Type: "invite_request",
		Data: InviteData{
			FromNickname: nickname,
		},
	})

	// Cria um canal para o convite e aguarda a resposta
	inviteChan := make(chan bool)
	inviteWait[opponentNickname] = inviteChan

	select {
	case accepted := <-inviteChan:
		if accepted {
			// Inicia o jogo
			sendMessage(player.Conn, Message{
				Type: "game_start",
				Data: map[string]string{
					"message": "The game has started. Please enter your move: rock, paper, or scissors.",
				},
			})
			sendMessage(opponent.Conn, Message{
				Type: "game_start",
				Data: map[string]string{
					"message": "The game has started. Please enter your move: rock, paper, or scissors.",
				},
			})

			startGame(player, opponent)

		} else {
			// Envia mensagem de rejeição
			sendMessage(conn, Message{
				Type: "invite_rejected",
				Data: map[string]string{
					"message": "Player " + opponentNickname + " rejected your invite.",
				},
			})
		}
	case <-time.After(15 * time.Second):
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
	case <-time.After(15 * time.Second):
		endGameDueToTimeout(player1, player2)
		return
	}

	select {
	case move2 := <-moveChan2:
		player2.Move = move2
	case <-time.After(15 * time.Second):
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
	conn.Write(append(messageBytes, '\n'))
}
