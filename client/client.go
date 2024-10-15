package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
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

var inGame bool
var currentInvite *Invite            // Variável para armazenar o convite atual
var inputChannel = make(chan string) // Canal para simular entrada
var resultChannel = make(chan bool)

const serverAddress = "localhost:8080"

func main() {
	// Conecta ao servidor
	conn, err := net.Dial("tcp", serverAddress)
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
	go listenForMessages(conn, nickname)

	// Goroutine para ler a entrada do usuário
	go func() {
		for {
			input := readInput()
			inputChannel <- input // Envia a entrada para o canal
		}
	}()

	for {
		// clearConsole()
		fmt.Println("\nMenu:")
		fmt.Println("1. Convidar Oponente")
		fmt.Println("2. Responder a Convite")
		fmt.Println("3. Sair")
		fmt.Print("Escolha uma opção: ")

		option := <-inputChannel // Lê a entrada do canal

		switch option {
		case "1":
			fmt.Print("Digite o nickname do oponente: ")
			opponent := <-inputChannel // Lê a entrada do canal

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

		case "4":
			clearInputChannel() // Limpa o canal de entradas antigas
			clearConsole()
			fmt.Println("====== Jogo começou!!! ======")
			fmt.Println("Escolha sua jogada: pedra, papel ou tesoura")

			move := <-inputChannel

			fmt.Println("Você escolheu: ", move)

			sendMessage(conn, Message{
				Type: "move",
				Data: map[string]string{
					"move":     move,
					"nickname": nickname,
				},
			})

			<-resultChannel

		case "5":
			continue

		default:
			fmt.Println("Opção inválida, tente novamente.")
		}
	}
}

func clearInputChannel() {
	for {
		select {
		case <-inputChannel:
		default:
			return
		}
	}
}

func listenForMessages(conn net.Conn, nickname string) {
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

		handleServerMessage(conn, message, nickname)
	}
}

func handleServerMessage(conn net.Conn, message Message, nickname string) {
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
		} else {
			fmt.Println("Aguardando resposta do oponente...")
		}

	case "invite_request":
		data := message.Data.(map[string]interface{})
		fromNickname := data["from_nickname"].(string)
		requestID := data["request_id"].(string)

		// Armazena o convite recebido
		currentInvite = &Invite{FromNickname: fromNickname, RequestID: requestID}
		fmt.Printf("Convite recebido de %s. Clique 2 para responder.\n", fromNickname)

	case "invite_rejected":
		inputChannel <- "5"

	case "game_start":
		inGame = true

		// Simula a opção 4 (iniciar o jogo)
		inputChannel <- "4" // Envia "4" para o canal, como se o usuário tivesse digitado

	case "game_result":
		data := message.Data.(map[string]interface{})

		var player1Nickname, player2Nickname, player1Move, player2Move string
		var winner string

		for nickname, move := range data {
			if nickname == "winner" {
				winner = move.(string)
			} else {
				if player1Nickname == "" {
					player1Nickname = nickname
					player1Move = move.(string)
				} else {
					player2Nickname = nickname
					player2Move = move.(string)
				}
			}
		}

		// Exibindo o resultado do jogo
		fmt.Printf("%s escolheu: %s, %s escolheu: %s. Vencedor: %s\n", player1Nickname, player1Move, player2Nickname, player2Move, winner)

		// Encerrando a conexão
		sendMessage(conn, Message{
			Type: "disconnect",
			Data: map[string]string{
				"nickname": nickname,
			},
		})
		os.Exit(0)

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

	response := <-inputChannel // Lê a resposta do canal
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
			Data: map[string]interface{}{
				"request_id": currentInvite.RequestID,
				"accepted":   false,
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

func clearConsole() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}
