#ifndef CSFML_ALL_H
#define CSFML_ALL_H

// CSFML - C bindings for SFML (Simple and Fast Multimedia Library)
// This header includes all CSFML modules

// System module - Core system functionality
#include <SFML/System.h>
#include <SFML/System/Clock.h>
#include <SFML/System/InputStream.h>
#include <SFML/System/Mutex.h>
#include <SFML/System/Sleep.h>
#include <SFML/System/Thread.h>
#include <SFML/System/Time.h>
#include <SFML/System/Vector2.h>
#include <SFML/System/Vector3.h>

// Window module - Window management and input handling
#include <SFML/Window.h>
#include <SFML/Window/Clipboard.h>
#include <SFML/Window/Context.h>
#include <SFML/Window/Cursor.h>
#include <SFML/Window/Event.h>
#include <SFML/Window/Joystick.h>
#include <SFML/Window/Keyboard.h>
#include <SFML/Window/Mouse.h>
#include <SFML/Window/Sensor.h>
#include <SFML/Window/Touch.h>
#include <SFML/Window/VideoMode.h>
#include <SFML/Window/Window.h>

// Graphics module - 2D graphics rendering
#include <SFML/Graphics.h>
#include <SFML/Graphics/BlendMode.h>
#include <SFML/Graphics/CircleShape.h>
#include <SFML/Graphics/Color.h>
#include <SFML/Graphics/ConvexShape.h>
#include <SFML/Graphics/Drawable.h>
#include <SFML/Graphics/Font.h>
#include <SFML/Graphics/Glsl.h>
#include <SFML/Graphics/Glyph.h>
#include <SFML/Graphics/Image.h>
#include <SFML/Graphics/PrimitiveType.h>
#include <SFML/Graphics/Rect.h>
#include <SFML/Graphics/RectangleShape.h>
#include <SFML/Graphics/RenderStates.h>
#include <SFML/Graphics/RenderTexture.h>
#include <SFML/Graphics/RenderWindow.h>
#include <SFML/Graphics/Shader.h>
#include <SFML/Graphics/Shape.h>
#include <SFML/Graphics/Sprite.h>
#include <SFML/Graphics/Text.h>
#include <SFML/Graphics/Texture.h>
#include <SFML/Graphics/Transform.h>
#include <SFML/Graphics/Transformable.h>
#include <SFML/Graphics/Vertex.h>
#include <SFML/Graphics/VertexArray.h>
#include <SFML/Graphics/View.h>

// Audio module - Audio playback and recording
#include <SFML/Audio.h>
#include <SFML/Audio/AlResource.h>
#include <SFML/Audio/Export.h>
#include <SFML/Audio/Listener.h>
#include <SFML/Audio/Music.h>
#include <SFML/Audio/Sound.h>
#include <SFML/Audio/SoundBuffer.h>
#include <SFML/Audio/SoundBufferRecorder.h>
#include <SFML/Audio/SoundRecorder.h>
#include <SFML/Audio/SoundStatus.h>
#include <SFML/Audio/SoundStream.h>
#include <SFML/Audio/Types.h>

// Network module - Network communication
#include <SFML/Network.h>
#include <SFML/Network/Export.h>
#include <SFML/Network/Ftp.h>
#include <SFML/Network/Http.h>
#include <SFML/Network/IpAddress.h>
#include <SFML/Network/Packet.h>
#include <SFML/Network/Socket.h>
#include <SFML/Network/SocketSelector.h>
#include <SFML/Network/TcpListener.h>
#include <SFML/Network/TcpSocket.h>
#include <SFML/Network/UdpSocket.h>

#endif // CSFML_ALL_H 