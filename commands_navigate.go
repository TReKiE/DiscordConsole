/*
DiscordConsole is a software aiming to give you full control over accounts, bots and webhooks!
Copyright (C) 2020 Mnpn

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
package main

import (
	"io"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/jD91mZM2/gtable"
	"github.com/jD91mZM2/stdutil"
)

func commandsNavigate(session *discordgo.Session, cmd string, args []string, nargs int, w io.Writer) (returnVal string) {
	switch cmd {
	case "guilds", "servers":
		cache, ok := <-chanReady
		if !ok && cacheGuilds != nil {
			mutexCacheGuilds.RLock()
			defer mutexCacheGuilds.RUnlock()

			cache = cacheGuilds
		}

		var guilds []*discordgo.UserGuild
		if cache != nil {
			guilds = cache
		} else {
			var err error
			guilds, err = session.UserGuilds(100, "", "", false)
			if err != nil {
				stdutil.PrintErr(tl("failed.guild"), err)
				return
			}

			mutexCacheGuilds.Lock()
			cacheGuilds = guilds
			mutexCacheGuilds.Unlock()
		}

		table := gtable.NewStringTable()
		table.AddStrings("ID", "Name")

		for _, guild := range guilds {
			table.AddRow()
			table.AddStrings(guild.ID, guild.Name)
		}

		writeln(w, table.String())
	case "guild", "server":
		if nargs < 1 {
			stdutil.PrintErr(cmd+" <id>", nil)
			return
		}

		guildID := strings.Join(args, " ")
		for _, g := range cacheGuilds {
			if strings.EqualFold(guildID, g.Name) {
				guildID = g.ID
				break
			}
		}

		guild, err := session.Guild(guildID)
		if err != nil {
			stdutil.PrintErr(tl("failed.guild"), err)
			return
		}

		channels, err := session.GuildChannels(guildID)
		if err != nil {
			stdutil.PrintErr(tl("failed.channel"), err)
			return
		}

		var channel *discordgo.Channel
		for _, channel2 := range channels {
			if channel2.Position == 0 {
				channel = channel2
			}
		}
		if channel == nil {
			stdutil.PrintErr(tl("failed.nochannel"), err)
			return
		}

		loc.push(guild, channel)
	case "channels":
		channels(session, discordgo.ChannelTypeGuildText, w)
	case "vchannels":
		channels(session, discordgo.ChannelTypeGuildVoice, w)
	case "pchannels":
		channels := session.State.Ready.PrivateChannels

		table := gtable.NewStringTable()
		table.AddStrings("ID", "Type", "Recipient(s)")

		for _, channel := range channels {
			table.AddRow()
			recipient := ""
			if len(channel.Recipients) > 0 {
				if len(channel.Recipients[0].Username) > 1 {
					recipient = channel.Recipients[0].Username
					if len(channel.Recipients) > 1 {
						for _, user := range channel.Recipients[1:] {
							recipient += ", " + user.Username
						}
					}
				}
			}
			kind := "DM"
			if channel.Type == discordgo.ChannelTypeGroupDM {
				if len(channel.Recipients) == 0 {
					kind = "Empty Group"
				} else {
					kind = "Group"
				}
			}
			table.AddStrings(channel.ID, kind, recipient)
		}
		writeln(w, table.String())
	case "channel":
		if nargs < 1 {
			stdutil.PrintErr("channel <id>", nil)
			return
		}

		arg := strings.Join(args, " ")

		var channel *discordgo.Channel
		for _, c := range cacheChannels {
			if strings.EqualFold(arg, c.Name) {
				channel = c
			}
		}
		if channel == nil {
			var err error
			channel, err = session.Channel(arg)
			if err != nil {
				stdutil.PrintErr(tl("failed.channel"), err)
				return
			}
		}

		if isPrivate(channel) {
			loc.push(nil, channel)
		} else {
			if loc.guild == nil || channel.GuildID != loc.guild.ID {
				guild, err := session.Guild(channel.GuildID)

				if err != nil {
					stdutil.PrintErr(tl("failed.guild"), err)
					return
				}

				loc.push(guild, channel)
			} else {
				loc.push(loc.guild, channel)
			}
		}
	case "dm":
		if nargs < 1 {
			stdutil.PrintErr("dm <user id>", nil)
			return
		}
		channel, err := session.UserChannelCreate(args[0])
		if err != nil {
			stdutil.PrintErr(tl("failed.channel.create"), err)
			return
		}
		loc.push(nil, channel)

		writeln(w, tl("status.channel")+" "+channel.ID)
	case "bookmarks":
		for key := range bookmarks {
			writeln(w, key)
		}
	case "bookmark":
		if nargs < 1 {
			stdutil.PrintErr("bookmark <name>", nil)
			return
		}

		name := strings.ToLower(strings.Join(args, " "))
		if strings.HasPrefix(name, "-") {
			name = name[1:]
			delete(bookmarks, name)
			delete(bookmarksCache, name)
		} else {
			bookmarks[name] = loc.channel.ID
			bookmarksCache[name] = loc
		}
		err := saveBookmarks()
		if err != nil {
			stdutil.PrintErr(tl("failed.file.save"), err)
		}
	case "go":
		if nargs < 1 {
			stdutil.PrintErr("go <bookmark>", nil)
			return
		}
		name := strings.ToLower(strings.Join(args, " "))
		if cache, ok := bookmarksCache[name]; ok {
			loc.push(cache.guild, cache.channel)
			return
		}

		bookmark, ok := bookmarks[name]
		if !ok {
			stdutil.PrintErr(tl("invalid.bookmark"), nil)
			return
		}

		var guild *discordgo.Guild
		var channel *discordgo.Channel
		var err error

		if bookmark != "" {
			channel, err = session.Channel(bookmark)
			if err != nil {
				stdutil.PrintErr(tl("failed.channel"), err)
				return
			}
		}

		if channel != nil && !isPrivate(channel) {
			guild, err = session.Guild(channel.GuildID)
			if err != nil {
				stdutil.PrintErr(tl("failed.guild"), err)
				return
			}
		}

		bookmarksCache[name] = &location{
			guild:   guild,
			channel: channel,
		}

		loc.push(guild, channel)
	}
	return
}

func channels(session *discordgo.Session, kind discordgo.ChannelType, w io.Writer) {
	var channels []*discordgo.Channel
	if cacheChannels != nil && cachedChannelType == kind {
		channels = cacheChannels
	} else {
		if loc.guild == nil {
			stdutil.PrintErr(tl("invalid.guild"), nil)
			return
		}
		channels2, err := session.GuildChannels(loc.guild.ID)
		if err != nil {
			stdutil.PrintErr(tl("failed.channel"), nil)
			return
		}

		cacheChannels = channels
		cachedChannelType = kind

		channels = make([]*discordgo.Channel, 0)
		for _, c := range channels2 {
			if c.Type != kind {
				continue
			}
			channels = append(channels, c)
		}

		sort.Slice(channels, func(i int, j int) bool {
			return channels[i].Position < channels[j].Position
		})

		cacheChannels = channels
		cachedChannelType = kind
	}

	table := gtable.NewStringTable()
	table.AddStrings("ID", "Name")

	for _, channel := range channels {
		table.AddRow()
		table.AddStrings(channel.ID, channel.Name)
	}

	writeln(w, table.String())
}
