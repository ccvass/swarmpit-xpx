package swarmpit.socket;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.*;
import java.nio.channels.Channels;
import java.nio.channels.SocketChannel;
import java.nio.file.Path;

public class HttpUnixSocket extends Socket {

    private final SocketChannel channel;
    private final boolean connected;

    public HttpUnixSocket(Path socketPath) throws IOException {
        this.channel = SocketChannel.open(StandardProtocolFamily.UNIX);
        this.channel.connect(UnixDomainSocketAddress.of(socketPath));
        this.connected = true;
    }

    @Override
    public void connect(SocketAddress endpoint, int timeout) throws IOException {
        // Already connected in constructor
    }

    @Override
    public void connect(SocketAddress endpoint) throws IOException {
        // Already connected in constructor
    }

    @Override
    public InputStream getInputStream() throws IOException {
        return Channels.newInputStream(channel);
    }

    @Override
    public OutputStream getOutputStream() throws IOException {
        return Channels.newOutputStream(channel);
    }

    @Override
    public boolean isConnected() {
        return connected && channel.isOpen();
    }

    @Override
    public boolean isClosed() {
        return !channel.isOpen();
    }

    @Override
    public synchronized void close() throws IOException {
        channel.close();
    }

    @Override
    public void shutdownInput() throws IOException {
        channel.shutdownInput();
    }

    @Override
    public void shutdownOutput() throws IOException {
        channel.shutdownOutput();
    }

    @Override public void setTcpNoDelay(boolean on) {}
    @Override public boolean getTcpNoDelay() { return false; }
    @Override public void setSoLinger(boolean on, int linger) {}
    @Override public int getSoLinger() { return -1; }
    @Override public void setKeepAlive(boolean on) {}
    @Override public boolean getKeepAlive() { return false; }
    @Override public void setReuseAddress(boolean on) {}
    @Override public boolean getReuseAddress() { return false; }
    @Override public synchronized void setSoTimeout(int timeout) {}
    @Override public synchronized int getSoTimeout() { return 0; }

    @Override
    public String toString() {
        return "HttpUnixSocket[connected=" + connected + "]";
    }
}
