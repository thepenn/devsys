package logger

import (
	"errors"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func InitLogging(logLevel string, pretty bool, outputLvl bool) error {
	if logLevel == "" {
		return errors.New("logLevel is empty")
	}

	output := os.Stderr
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

	if pretty {
		log.Logger = log.Output(
			zerolog.ConsoleWriter{
				Out:     output,
				NoColor: false,
			},
		)
	}

	lvl, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(lvl)

	if zerolog.GlobalLevel() <= zerolog.DebugLevel {
		log.Logger = log.With().Caller().Logger()
	}

	if outputLvl {
		log.Info().Msgf("Log level: %s", zerolog.GlobalLevel().String())
	}

	return nil
}
