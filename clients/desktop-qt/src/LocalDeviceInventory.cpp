#include "LocalDeviceInventory.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMap>
#include <QProcess>
#include <QRegularExpression>
#include <QSet>

#include <algorithm>

namespace {

QString readTrimmed(const QString &path) {
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
        return {};
    }
    return QString::fromUtf8(file.readAll()).trimmed();
}

QString basenameOfLink(const QString &path) {
    QFileInfo info(path);
    const QString target = info.symLinkTarget();
    if (target.isEmpty()) {
        return {};
    }
    return QFileInfo(target).fileName();
}

QJsonObject serialRuleFor(const QString &portName, const QString &driver) {
    const QString lowerPort = portName.toLower();
    const QString lowerDriver = driver.toLower();

    QJsonObject rule{
        {QStringLiteral("driver"), driver},
        {QStringLiteral("transport"), QStringLiteral("tcp-encap")},
        {QStringLiteral("suggested_baud_rate"), 115200},
        {QStringLiteral("priority"), 50},
        {QStringLiteral("rule"), QStringLiteral("通用串口设备，建议先走 tcp-encap，并确认宿主机未被其它进程占用")},
    };

    if (lowerDriver.contains(QStringLiteral("cdc_acm")) || lowerPort.contains(QStringLiteral("ttyacm"))) {
        rule[QStringLiteral("driver")] = QStringLiteral("cdc_acm");
        rule[QStringLiteral("priority")] = 95;
        rule[QStringLiteral("rule")] = QStringLiteral("CDC ACM 设备，常见开发板/工业控制器，默认建议 115200，优先检查 ModemManager 等占用");
        return rule;
    }
    if (lowerDriver.contains(QStringLiteral("cp210")) || lowerDriver.contains(QStringLiteral("silabs"))) {
        rule[QStringLiteral("driver")] = QStringLiteral("cp210x");
        rule[QStringLiteral("priority")] = 92;
        rule[QStringLiteral("rule")] = QStringLiteral("Silicon Labs CP210x USB 转串口，默认建议 115200，适合先走 tcp-encap");
        return rule;
    }
    if (lowerDriver.contains(QStringLiteral("ftdi"))) {
        rule[QStringLiteral("driver")] = QStringLiteral("ftdi_sio");
        rule[QStringLiteral("priority")] = 91;
        rule[QStringLiteral("rule")] = QStringLiteral("FTDI USB 转串口，默认建议 115200，若存在多端口设备请确认通道编号");
        return rule;
    }
    if (lowerDriver.contains(QStringLiteral("ch34")) || lowerDriver.contains(QStringLiteral("ch341")) || lowerDriver.contains(QStringLiteral("wch"))) {
        rule[QStringLiteral("driver")] = QStringLiteral("ch341");
        rule[QStringLiteral("priority")] = 90;
        rule[QStringLiteral("rule")] = QStringLiteral("CH340/CH341 USB 转串口，默认建议 115200，转发前确认宿主机串口权限");
        return rule;
    }
    if (lowerDriver.contains(QStringLiteral("pl2303"))) {
        rule[QStringLiteral("driver")] = QStringLiteral("pl2303");
        rule[QStringLiteral("priority")] = 88;
        rule[QStringLiteral("rule")] = QStringLiteral("PL2303 USB 转串口，默认建议 115200，转发前确认线缆与芯片兼容性");
        return rule;
    }
    if (lowerPort.contains(QStringLiteral("ttyusb"))) {
        rule[QStringLiteral("priority")] = 85;
        rule[QStringLiteral("rule")] = QStringLiteral("USB 串口设备，默认建议 115200，若设备有厂商驱动请优先确认当前驱动已稳定加载");
        return rule;
    }
    if (lowerPort.contains(QStringLiteral("ttys")) || lowerPort.contains(QStringLiteral("com"))) {
        rule[QStringLiteral("suggested_baud_rate")] = 9600;
        rule[QStringLiteral("priority")] = 70;
        rule[QStringLiteral("rule")] = QStringLiteral("板载串口/传统 COM 口，默认建议 9600，转发前确认 BIOS/宿主机未独占");
        return rule;
    }
    return rule;
}

QJsonObject usbRuleFor(const QString &deviceClass, const QString &driver) {
    const QString normalizedClass = deviceClass.toLower();
    const QString normalizedDriver = driver.toLower();
    QJsonObject rule{
        {QStringLiteral("driver"), driver},
        {QStringLiteral("transport"), QStringLiteral("usbip-encap")},
        {QStringLiteral("priority"), 50},
        {QStringLiteral("rule"), QStringLiteral("通用 USB 设备，建议先确认宿主机驱动稳定，再使用 usbip-encap 转发")},
    };

    if (normalizedDriver.contains(QStringLiteral("usbhid")) || normalizedClass == QStringLiteral("03")) {
        rule[QStringLiteral("priority")] = 35;
        rule[QStringLiteral("rule")] = QStringLiteral("HID 设备，宿主机通常会自动接管，转发前需确认独占策略和输入法/桌面占用");
        return rule;
    }
    if (normalizedDriver.contains(QStringLiteral("usb-storage")) || normalizedClass == QStringLiteral("08")) {
        rule[QStringLiteral("priority")] = 20;
        rule[QStringLiteral("rule")] = QStringLiteral("存储设备，转发前必须确认宿主机未挂载文件系统，否则容易出现数据损坏");
        return rule;
    }
    if (normalizedDriver.contains(QStringLiteral("hub")) || normalizedClass == QStringLiteral("09")) {
        rule[QStringLiteral("priority")] = 5;
        rule[QStringLiteral("rule")] = QStringLiteral("USB Hub 或根集线器，不建议作为转发目标");
        return rule;
    }
    if (normalizedClass == QStringLiteral("02") || normalizedClass == QStringLiteral("0a")) {
        rule[QStringLiteral("priority")] = 85;
        rule[QStringLiteral("rule")] = QStringLiteral("通信类 USB 设备，可优先尝试 usbip-encap，注意宿主机调制解调器管理服务");
        return rule;
    }
    if (normalizedClass == QStringLiteral("07")) {
        rule[QStringLiteral("priority")] = 80;
        rule[QStringLiteral("rule")] = QStringLiteral("打印类设备，可尝试 usbip-encap，转发前确认宿主机打印队列为空");
        return rule;
    }
    if (normalizedClass == QStringLiteral("ff")) {
        rule[QStringLiteral("priority")] = 88;
        rule[QStringLiteral("rule")] = QStringLiteral("厂商自定义 USB 设备，若原厂驱动已加载稳定，可优先尝试 usbip-encap");
        return rule;
    }
    return rule;
}

QJsonArray sortObjects(QList<QJsonObject> items) {
    std::sort(items.begin(), items.end(), [](const QJsonObject &left, const QJsonObject &right) {
        const int leftPriority = left.value(QStringLiteral("priority")).toInt();
        const int rightPriority = right.value(QStringLiteral("priority")).toInt();
        if (leftPriority != rightPriority) {
            return leftPriority > rightPriority;
        }
        return left.value(QStringLiteral("display_name")).toString() < right.value(QStringLiteral("display_name")).toString();
    });

    QJsonArray array;
    for (const QJsonObject &item : items) {
        array.append(item);
    }
    return array;
}

#ifdef Q_OS_LINUX
QJsonArray enumerateLinuxSerialDevices() {
    const QStringList patterns{
        QStringLiteral("ttyUSB*"),
        QStringLiteral("ttyACM*"),
        QStringLiteral("ttyAMA*"),
        QStringLiteral("ttyS*"),
        QStringLiteral("rfcomm*"),
    };
    QDir devDir(QStringLiteral("/dev"));
    QFileInfoList entries;
    for (const QString &pattern : patterns) {
        entries.append(devDir.entryInfoList({pattern}, QDir::System | QDir::Files | QDir::Readable, QDir::Name));
    }

    QMap<QString, QString> byIdAliases;
    const QDir byIdDir(QStringLiteral("/dev/serial/by-id"));
    for (const QFileInfo &info : byIdDir.entryInfoList(QDir::System | QDir::Files | QDir::NoDotAndDotDot, QDir::Name)) {
        byIdAliases.insert(QFileInfo(info.symLinkTarget()).fileName(), info.fileName());
    }

    QList<QJsonObject> results;
    QSet<QString> seen;
    for (const QFileInfo &entry : entries) {
        const QString portPath = entry.absoluteFilePath();
        if (seen.contains(portPath)) {
            continue;
        }
        seen.insert(portPath);
        const QString ttyName = entry.fileName();
        QString driver = basenameOfLink(QStringLiteral("/sys/class/tty/%1/device/driver").arg(ttyName));
        if (driver.isEmpty()) {
            driver = basenameOfLink(QStringLiteral("/sys/class/tty/%1/device/../driver").arg(ttyName));
        }
        const QJsonObject rule = serialRuleFor(portPath, driver);
        QString displayName = portPath;
        const QString alias = byIdAliases.value(ttyName);
        if (!alias.isEmpty()) {
            displayName = QStringLiteral("%1 (%2)").arg(alias, portPath);
        }
        QJsonObject object{
            {QStringLiteral("port_name"), portPath},
            {QStringLiteral("display_name"), displayName},
            {QStringLiteral("driver"), rule.value(QStringLiteral("driver")).toString()},
            {QStringLiteral("transport"), rule.value(QStringLiteral("transport")).toString()},
            {QStringLiteral("suggested_baud_rate"), rule.value(QStringLiteral("suggested_baud_rate")).toInt()},
            {QStringLiteral("priority"), rule.value(QStringLiteral("priority")).toInt()},
            {QStringLiteral("rule"), rule.value(QStringLiteral("rule")).toString()},
        };
        results.append(object);
    }
    return sortObjects(results);
}

QString findUsbInterfaceDriver(const QFileInfo &deviceInfo, QString *deviceClass) {
    QDir deviceDir(deviceInfo.absoluteFilePath());
    const QFileInfoList interfaces = deviceDir.entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name);
    for (const QFileInfo &entry : interfaces) {
        if (!entry.fileName().contains(':')) {
            continue;
        }
        if (deviceClass) {
            const QString cls = readTrimmed(entry.absoluteFilePath() + QStringLiteral("/bInterfaceClass"));
            if (!cls.isEmpty()) {
                *deviceClass = cls;
            }
        }
        const QString driver = basenameOfLink(entry.absoluteFilePath() + QStringLiteral("/driver"));
        if (!driver.isEmpty()) {
            return driver;
        }
    }
    return {};
}

QJsonArray enumerateLinuxUsbDevices() {
    QDir sysDir(QStringLiteral("/sys/bus/usb/devices"));
    const QFileInfoList entries = sysDir.entryInfoList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name);
    QList<QJsonObject> results;

    for (const QFileInfo &entry : entries) {
        const QString base = entry.absoluteFilePath();
        const QString vendor = readTrimmed(base + QStringLiteral("/idVendor"));
        const QString product = readTrimmed(base + QStringLiteral("/idProduct"));
        if (vendor.isEmpty() || product.isEmpty()) {
            continue;
        }

        QString deviceClass;
        const QString driver = findUsbInterfaceDriver(entry, &deviceClass);
        const QJsonObject rule = usbRuleFor(deviceClass, driver);

        const QString busNum = readTrimmed(base + QStringLiteral("/busnum"));
        const QString devNum = readTrimmed(base + QStringLiteral("/devnum"));
        const QString manufacturer = readTrimmed(base + QStringLiteral("/manufacturer"));
        const QString productName = readTrimmed(base + QStringLiteral("/product"));
        const QString serial = readTrimmed(base + QStringLiteral("/serial"));
        const QString displayName = QStringLiteral("Bus %1 Device %2 %3:%4 %5 %6")
                                        .arg(busNum.isEmpty() ? QStringLiteral("?") : busNum,
                                             devNum.isEmpty() ? QStringLiteral("?") : devNum,
                                             vendor,
                                             product,
                                             manufacturer,
                                             productName)
                                        .trimmed();

        QJsonObject object{
            {QStringLiteral("bus_id"), busNum},
            {QStringLiteral("device_id"), devNum},
            {QStringLiteral("vendor_id"), vendor},
            {QStringLiteral("product_id"), product},
            {QStringLiteral("interface"), deviceClass},
            {QStringLiteral("serial_number"), serial},
            {QStringLiteral("product_name"), productName},
            {QStringLiteral("display_name"), displayName},
            {QStringLiteral("driver"), rule.value(QStringLiteral("driver")).toString()},
            {QStringLiteral("transport"), rule.value(QStringLiteral("transport")).toString()},
            {QStringLiteral("priority"), rule.value(QStringLiteral("priority")).toInt()},
            {QStringLiteral("rule"), rule.value(QStringLiteral("rule")).toString()},
        };
        results.append(object);
    }

    return sortObjects(results);
}
#endif

#ifdef Q_OS_WIN
QJsonArray enumerateWindowsSerialDevices() {
    QProcess process;
    process.start(QStringLiteral("powershell"), {
        QStringLiteral("-NoProfile"),
        QStringLiteral("-Command"),
        QStringLiteral("Get-CimInstance Win32_SerialPort | Select-Object DeviceID,Name,Description | ConvertTo-Json -Depth 3 -Compress"),
    });
    process.waitForFinished(4000);
    const QJsonDocument doc = QJsonDocument::fromJson(process.readAllStandardOutput());
    QList<QJsonObject> results;
    const QJsonArray items = doc.isArray() ? doc.array() : QJsonArray{doc.object()};
    for (const QJsonValue &value : items) {
        const QJsonObject input = value.toObject();
        const QString portName = input.value(QStringLiteral("DeviceID")).toString();
        const QString description = input.value(QStringLiteral("Description")).toString();
        QJsonObject rule = serialRuleFor(portName, description);
        QJsonObject object{
            {QStringLiteral("port_name"), portName},
            {QStringLiteral("display_name"), input.value(QStringLiteral("Name")).toString()},
            {QStringLiteral("driver"), rule.value(QStringLiteral("driver")).toString()},
            {QStringLiteral("transport"), rule.value(QStringLiteral("transport")).toString()},
            {QStringLiteral("suggested_baud_rate"), rule.value(QStringLiteral("suggested_baud_rate")).toInt()},
            {QStringLiteral("priority"), rule.value(QStringLiteral("priority")).toInt()},
            {QStringLiteral("rule"), rule.value(QStringLiteral("rule")).toString()},
        };
        results.append(object);
    }
    return sortObjects(results);
}

QJsonArray enumerateWindowsUsbDevices() {
    QProcess process;
    process.start(QStringLiteral("powershell"), {
        QStringLiteral("-NoProfile"),
        QStringLiteral("-Command"),
        QStringLiteral("Get-PnpDevice -PresentOnly | Where-Object { $_.InstanceId -like 'USB*' } | Select-Object FriendlyName,InstanceId,Class,Service | ConvertTo-Json -Depth 3 -Compress"),
    });
    process.waitForFinished(5000);
    const QJsonDocument doc = QJsonDocument::fromJson(process.readAllStandardOutput());
    QList<QJsonObject> results;
    const QJsonArray items = doc.isArray() ? doc.array() : QJsonArray{doc.object()};
    const QRegularExpression regex(QStringLiteral("VID_([0-9A-Fa-f]{4}).*PID_([0-9A-Fa-f]{4})"));
    for (const QJsonValue &value : items) {
        const QJsonObject input = value.toObject();
        const QString instanceId = input.value(QStringLiteral("InstanceId")).toString();
        const QRegularExpressionMatch match = regex.match(instanceId);
        const QString vendor = match.hasMatch() ? match.captured(1).toLower() : QString();
        const QString product = match.hasMatch() ? match.captured(2).toLower() : QString();
        const QString deviceClass = input.value(QStringLiteral("Class")).toString();
        const QString driver = input.value(QStringLiteral("Service")).toString();
        const QJsonObject rule = usbRuleFor(deviceClass, driver);
        QJsonObject object{
            {QStringLiteral("bus_id"), QString()},
            {QStringLiteral("device_id"), QString()},
            {QStringLiteral("vendor_id"), vendor},
            {QStringLiteral("product_id"), product},
            {QStringLiteral("interface"), deviceClass},
            {QStringLiteral("serial_number"), instanceId},
            {QStringLiteral("product_name"), input.value(QStringLiteral("FriendlyName")).toString()},
            {QStringLiteral("display_name"), input.value(QStringLiteral("FriendlyName")).toString()},
            {QStringLiteral("driver"), rule.value(QStringLiteral("driver")).toString()},
            {QStringLiteral("transport"), rule.value(QStringLiteral("transport")).toString()},
            {QStringLiteral("priority"), rule.value(QStringLiteral("priority")).toInt()},
            {QStringLiteral("rule"), rule.value(QStringLiteral("rule")).toString()},
        };
        results.append(object);
    }
    return sortObjects(results);
}
#endif

} // namespace

QJsonArray LocalDeviceInventory::enumerateSerialDevices() {
#ifdef Q_OS_LINUX
    return enumerateLinuxSerialDevices();
#elif defined(Q_OS_WIN)
    return enumerateWindowsSerialDevices();
#else
    return {};
#endif
}

QJsonArray LocalDeviceInventory::enumerateUsbDevices() {
#ifdef Q_OS_LINUX
    return enumerateLinuxUsbDevices();
#elif defined(Q_OS_WIN)
    return enumerateWindowsUsbDevices();
#else
    return {};
#endif
}
