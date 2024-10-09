package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Estrutura da mensagem
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Estrutura para armazenar o convite
type Invite struct {
	FromNickname string
	RequestID    string
}

var currentInvite *Invite // Variável para armazenar o convite atual

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

	// Loop principal do cliente
	for {
		fmt.Println("\nMenu:")
		fmt.Println("1. Convidar Oponente")
		fmt.Println("2. Responder a Convite")
		fmt.Println("3. Sair")
		fmt.Print("Escolha uma opção: ")

		option := readInput()

		switch option {
		case "1":
			fmt.Print("Digite o nickname do oponente: ")
			opponent := readInput()

			// Envia a solicitação para iniciar o jogo com o oponente escolhido
			sendMessage(conn, Message{
				Type: "opponent_request",
				Data: map[string]string{
					"nickname":          nickname,
					"opponent_nickname": opponent,
				},
			})

		case "2":
			handleResponseToInvite(conn)

		case "3":
			fmt.Println("Saindo...")
			return

		default:
			fmt.Println("Opção inválida, tente novamente.")
		}
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
	fmt.Println("\n\nMensagem do servidor: ", message)
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
		requestID := data["request_id"].(string)

		// Armazena o convite recebido
		currentInvite = &Invite{FromNickname: fromNickname, RequestID: requestID}
		fmt.Printf("Convite recebido de %s. Você pode responder na opção 2.\n", fromNickname)

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

func handleResponseToInvite(conn net.Conn) {
	if currentInvite == nil {
		fmt.Println("Não há convites pendentes no momento.")
		return
	}

	// Responde ao convite armazenado
	fromNickname := currentInvite.FromNickname
	fmt.Printf("O jogador %s quer jogar com você. Aceitar? (s/n): ", fromNickname)

	response := readInput() // Responde ao convite
	if strings.ToLower(response) == "s" {
		sendMessage(conn, Message{
			Type: "invite_response",
			Data: map[string]interface{}{
				"request_id": currentInvite.RequestID,
				"accepted":   true,
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
	// Limpa o convite após responder
	currentInvite = nil
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
