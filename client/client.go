package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Estrutura da mensagem
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func main() {
	// Conecta ao servidor
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Erro ao conectar ao servidor:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Solicita o nickname do jogador
	fmt.Print("Digite seu nickname: ")
	nickname := readInput()

	// Envia a solicitação de conexão para o servidor com o nickname
	sendMessage(conn, Message{
		Type: "connect_request",
		Data: map[string]string{
			"nickname": nickname,
		},
	})

	// Recebe a confirmação do servidor antes de continuar
	go listenForMessages(conn)

	// Solicita o nickname do oponente após o próprio nickname ter sido enviado
	time.Sleep(1 * time.Second) // Pequena pausa para garantir que o servidor processe a primeira mensagem
	fmt.Print("Digite o nickname do oponente: ")
	opponent := readInput()

	// Envia a solicitação para iniciar o jogo com o oponente escolhido
	sendMessage(conn, Message{
		Type: "opponent_request",
		Data: map[string]string{
			"opponent_nickname": opponent,
		},
	})

	// Mantém o cliente rodando
	for {
		time.Sleep(1 * time.Second)
	}
}

func listenForMessages(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		messageStr, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Erro ao ler mensagem do servidor:", err)
			break
		}

		var message Message
		err = json.Unmarshal([]byte(messageStr), &message)
		if err != nil {
			fmt.Println("Erro ao decodificar mensagem:", err)
			continue
		}

		handleServerMessage(conn, message)
	}
}

func handleServerMessage(conn net.Conn, message Message) {
	fmt.Println("message do server: ", message)
	switch message.Type {
	case "connect_response":
		data := message.Data.(map[string]interface{})
		status := data["status"].(string)
		if status == "error" {
			fmt.Println("Erro:", data["message"])
			os.Exit(1)
		} else {
			fmt.Println("Conexão estabelecida com sucesso!")
		}

	case "opponent_response":
		data := message.Data.(map[string]interface{})
		status := data["status"].(string)
		if status == "error" {
			fmt.Println("Erro:", data["message"])
			os.Exit(1)
		} else {
			fmt.Println("Convite enviado para o oponente.")
		}

	case "invite_request":
		data := message.Data.(map[string]interface{})
		fromNickname := data["from_nickname"].(string)
		fmt.Printf("O jogador %s quer jogar com você. Aceitar? (s/n): ", fromNickname)
		response := readInput()
		if strings.ToLower(response) == "s" {
			sendMessage(conn, Message{
				Type: "invite_response",
				Data: map[string]bool{
					"accepted": true,
				},
			})
		} else {
			sendMessage(conn, Message{
				Type: "invite_response",
				Data: map[string]bool{
					"accepted": false,
				},
			})
		}

	case "game_start":
		fmt.Println("O jogo começou! Escolha sua jogada: pedra, papel ou tesoura")
		move := readInput()
		sendMessage(conn, Message{
			Type: "move",
			Data: map[string]string{
				"move": move,
			},
		})

	case "game_result":
		data := message.Data.(map[string]interface{})
		player1Move := data["player1_move"].(string)
		player2Move := data["player2_move"].(string)
		winner := data["winner"].(string)
		fmt.Printf("Jogador 1 escolheu: %s, Jogador 2 escolheu: %s. Vencedor: %s\n", player1Move, player2Move, winner)

	case "timeout":
		fmt.Println("O jogo terminou devido à inatividade.")
		os.Exit(0)
	}
}

func sendMessage(conn net.Conn, message Message) {
	messageBytes, _ := json.Marshal(message)
	conn.Write(append(messageBytes, '\n'))
}

func readInput() string {
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}
