package handlers

import "github.com/bwmarrin/discordgo"

// handleSlashCommand는 '/agent' 슬래시 명령어를 처리합니다.
func (h *DiscordHandler) handleSlashCommand(i *discordgo.InteractionCreate) {
	if i.ApplicationCommandData().Name != cmdAgent {
		return
	}
	subCommand := i.ApplicationCommandData().Options[0]
	switch subCommand.Name {
	case subCmdCreate:
		h.showCreateOrEditModal(i, "", nil)
	case subCmdList:
		h.showAgentList(i)
	case subCmdView:
		h.showAgentDetails(i, subCommand.Options[0].StringValue())
	case subCmdDelete:
		h.deleteAgent(i, subCommand.Options[0].StringValue())
	case subCmdEdit:
		h.showEditUI(i, subCommand.Options[0].StringValue())
	case subCmdCall:
		h.startAgentThread(i, subCommand.Options[0].StringValue())
	}
}
