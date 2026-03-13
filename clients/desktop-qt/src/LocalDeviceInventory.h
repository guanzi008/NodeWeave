#pragma once

#include <QJsonArray>

class LocalDeviceInventory {
public:
    static QJsonArray enumerateSerialDevices();
    static QJsonArray enumerateUsbDevices();
};
