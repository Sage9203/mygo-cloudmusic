# Synthetic audio regression fixtures

seek.flac and seek.mp3 contain five seconds of generated mathematical signals, not recordings or song excerpts. They replace upstream fixtures whose individual generation provenance was not recorded.

Generator: scripts/testaudio/main.go, maintained under the project's AGPLv3 license. The generated signals and encoded fixtures are also provided under the project's AGPLv3 license.

Both channels use different sums of sine waves, a chirp and a slow amplitude envelope at 44100 Hz, 16-bit stereo. FLAC is written with the existing github.com/mewkiz/flac dependency. MP3 is CBR 128 kbit/s with the bit reservoir enabled.

To regenerate, supply an existing LAME executable:

    go run ./scripts/testaudio -lame /path/to/lame

This run used LAME 3.100.1 compiled from the LAME project's GitHub mirror at commit 1f5cc9487284d5950343aa5d4f70de433468070a:
https://github.com/lameproject/lame/tree/1f5cc9487284d5950343aa5d4f70de433468070a

The encoder command is recorded in the generator. LAME is a generation-only tool, not an application dependency; neither its source nor binary is included in the normal application package. Regenerating fixtures is unnecessary for ordinary builds and tests.

The MP3 has 193 frames and reaches main_data_begin=511, including borrowing past the immediately preceding frame's main-data payload. This preserves the multi-frame bit-reservoir case that TestMP3SeekIsClean needs. structure.json records the generation check. Existing FLAC seek and metadata regression tests must continue to pass after regeneration. Encoder versions can change the compressed bytes; the project does not claim bit-identical encoding across versions.